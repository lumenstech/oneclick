package source

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Checkout struct {
	Path     string
	Ref      string
	Type     string
	Revision string
	Dirty    bool
	cleanup  func()
}

func (c Checkout) Close() {
	if c.cleanup != nil {
		c.cleanup()
	}
}

func Prepare(input string) (Checkout, error) {
	if st, err := os.Stat(input); err == nil && st.IsDir() {
		abs, err := filepath.Abs(input)
		if err != nil {
			return Checkout{}, err
		}
		rev, dirty := gitState(abs)
		return Checkout{Path: abs, Ref: abs, Type: "local", Revision: rev, Dirty: dirty}, nil
	}

	if strings.HasPrefix(input, "https://github.com/") || strings.HasPrefix(input, "git@github.com:") {
		tmp, err := os.MkdirTemp("", "oneclick-repo-*")
		if err != nil {
			return Checkout{}, err
		}
		cmd := exec.Command("git", "clone", "--depth", "1", "--", input, tmp)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			_ = os.RemoveAll(tmp)
			return Checkout{}, fmt.Errorf("git clone failed: %w: %s", err, strings.TrimSpace(string(out)))
		}
		rev, _ := gitState(tmp)
		return Checkout{Path: tmp, Ref: input, Type: "github", Revision: rev, Dirty: false, cleanup: func() { _ = os.RemoveAll(tmp) }}, nil
	}
	return Checkout{}, fmt.Errorf("source must be a local directory or GitHub repository URL")
}

func gitState(path string) (string, bool) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	rev := strings.TrimSpace(string(out))
	status := exec.Command("git", "-C", path, "status", "--porcelain", "--untracked-files=normal")
	statusOut, err := status.Output()
	if err != nil {
		return rev, false
	}
	return rev, strings.TrimSpace(string(statusOut)) != ""
}
