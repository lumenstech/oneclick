package node

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tarball(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, body := range entries {
		h := &tar.Header{Name: "prefix/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return b.Bytes()
}

func TestGitHubFetcherUsesHeaderAndExactCommit(t *testing.T) {
	archive := tarball(t, map[string]string{"Dockerfile": "FROM scratch\n"})
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	g := NewGitHubFetcher(srv.Client())
	g.APIBase = srv.URL
	dest := t.TempDir()
	sha := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := g.Fetch(context.Background(), "owner/repo", sha, "opaque-token", dest); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer opaque-token" {
		t.Fatalf("token not sent as header: %q", gotAuth)
	}
	if gotPath != "/repos/owner/repo/tarball/"+sha {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if _, err := os.Stat(filepath.Join(dest, "Dockerfile")); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubFetcherRejectsLFS(t *testing.T) {
	archive := tarball(t, map[string]string{"model.bin": "version https://git-lfs.github.com/spec/v1\noid sha256:abc\nsize 1\n"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archive) }))
	defer srv.Close()
	g := NewGitHubFetcher(srv.Client())
	g.APIBase = srv.URL
	if err := g.Fetch(context.Background(), "owner/repo", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "t", t.TempDir()); err == nil {
		t.Fatal("expected LFS rejection")
	}
}

func TestGitHubFetcherDoesNotForwardTokenAcrossArchiveRedirect(t *testing.T) {
	archive := tarball(t, map[string]string{"Dockerfile": "FROM scratch\n"})
	var downloadAuth string
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloadAuth = r.Header.Get("Authorization")
		_, _ = w.Write(archive)
	}))
	defer download.Close()
	var apiAuth string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiAuth = r.Header.Get("Authorization")
		http.Redirect(w, r, download.URL+"/temporary-archive", http.StatusFound)
	}))
	defer api.Close()

	g := NewGitHubFetcher(api.Client())
	g.APIBase = api.URL
	if err := g.Fetch(context.Background(), "owner/repo", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "opaque-install-token", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if apiAuth != "Bearer opaque-install-token" {
		t.Fatalf("API request missing token: %q", apiAuth)
	}
	if downloadAuth != "" {
		t.Fatalf("installation token leaked to archive redirect: %q", downloadAuth)
	}
}

func TestGitHubFetcherRejectsUnexpectedRedirectHost(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://example.invalid/steal")
		w.WriteHeader(http.StatusFound)
	}))
	defer api.Close()
	g := NewGitHubFetcher(api.Client())
	g.APIBase = api.URL
	err := g.Fetch(context.Background(), "owner/repo", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "opaque-install-token", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "redirect rejected") {
		t.Fatalf("expected redirect rejection, got %v", err)
	}
}
