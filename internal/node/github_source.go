package node

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type GitHubFetcher struct {
	Client             *http.Client
	APIBase            string
	MaxCompressedBytes int64
	MaxExpandedBytes   int64
	MaxFiles           int
}

func NewGitHubFetcher(client *http.Client) GitHubFetcher {
	return GitHubFetcher{Client: client, APIBase: "https://api.github.com", MaxCompressedBytes: 1 << 30, MaxExpandedBytes: 5 << 30, MaxFiles: 100000}
}

func (g GitHubFetcher) Fetch(ctx context.Context, repository, commitSHA, token, dest string) error {
	if !validRepo(repository) || !(isHexLen(commitSHA, 40) || isHexLen(commitSHA, 64)) {
		return fmt.Errorf("invalid GitHub source")
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("missing GitHub installation token")
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	archiveURL := strings.TrimRight(g.APIBase, "/") + "/repos/" + repository + "/tarball/" + commitSHA
	resp, err := g.openArchive(ctx, archiveURL, token)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub tarball failed: http %d", resp.StatusCode)
	}
	lr := &io.LimitedReader{R: resp.Body, N: g.MaxCompressedBytes + 1}
	gz, err := gzip.NewReader(lr)
	if err != nil {
		return fmt.Errorf("invalid GitHub tarball: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var expanded int64
	files := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if lr.N <= 0 {
			return fmt.Errorf("GitHub tarball exceeds compressed size limit")
		}
		parts := strings.SplitN(filepath.ToSlash(h.Name), "/", 2)
		if len(parts) != 2 || parts[1] == "" {
			continue
		}
		rel := filepath.Clean(filepath.FromSlash(parts[1]))
		if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe tar path")
		}
		target := filepath.Join(dest, rel)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe tar target")
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			files++
			if files > g.MaxFiles {
				return fmt.Errorf("GitHub tarball exceeds file count limit")
			}
			if h.Size < 0 || expanded+h.Size > g.MaxExpandedBytes {
				return fmt.Errorf("GitHub tarball exceeds expanded size limit")
			}
			expanded += h.Size
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.FileInfo().Mode()&0o111 != 0 {
				mode = 0o755
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			_, cpErr := io.CopyN(f, tr, h.Size)
			closeErr := f.Close()
			if cpErr != nil {
				return cpErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("repository archives with symlinks or hardlinks are not supported in node v0.1")
		default:
			// Ignore metadata-only tar records.
		}
	}
	if _, err := os.Stat(filepath.Join(dest, ".gitmodules")); err == nil {
		return fmt.Errorf("git submodules are not supported in node v0.1")
	}
	if lfs, err := containsLFSPointer(dest); err != nil {
		return err
	} else if lfs {
		return fmt.Errorf("Git LFS pointer detected; LFS repositories are not supported in node v0.1")
	}
	return nil
}

func (g GitHubFetcher) openArchive(ctx context.Context, archiveURL, token string) (*http.Response, error) {
	baseClient := g.Client
	if baseClient == nil {
		baseClient = http.DefaultClient
	}
	client := *baseClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "oneclick-node/"+Version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusFound {
		return resp, nil
	}
	location := resp.Header.Get("Location")
	_ = resp.Body.Close()
	redirect, err := url.Parse(location)
	if err != nil || !allowedArchiveRedirect(g.APIBase, redirect) {
		return nil, fmt.Errorf("GitHub archive redirect rejected")
	}
	second, err := http.NewRequestWithContext(ctx, http.MethodGet, redirect.String(), nil)
	if err != nil {
		return nil, err
	}
	second.Header.Set("Accept", "application/octet-stream")
	second.Header.Set("User-Agent", "oneclick-node/"+Version)
	// Do not forward the installation token. The Location URL is temporary and
	// already authorizes the archive download for a private repository.
	return client.Do(second)
}

func allowedArchiveRedirect(apiBase string, redirect *url.URL) bool {
	if redirect == nil || redirect.Hostname() == "" {
		return false
	}
	base, err := url.Parse(apiBase)
	if err != nil {
		return false
	}
	if strings.EqualFold(base.Hostname(), "api.github.com") {
		return redirect.Scheme == "https" && strings.EqualFold(redirect.Hostname(), "codeload.github.com")
	}
	// Test/development API bases may use a local HTTP server. Keep redirects on
	// the same scheme and hostname so an injected Location cannot become SSRF.
	return redirect.Scheme == base.Scheme && strings.EqualFold(redirect.Hostname(), base.Hostname())
}

func containsLFSPointer(root string) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > 1<<20 {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		b := make([]byte, 256)
		n, _ := f.Read(b)
		_ = f.Close()
		if strings.Contains(string(b[:n]), "version https://git-lfs.github.com/spec/v1") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found, err
}
