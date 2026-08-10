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
		return Checkout{Path: abs, Ref: abs, Type: "local", Revision: revision(abs)}, nil
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
		return Checkout{Path: tmp, Ref: input, Type: "github", Revision: revision(tmp), cleanup: func() { _ = os.RemoveAll(tmp) }}, nil
	}
	return Checkout{}, fmt.Errorf("source must be a local directory or GitHub repository URL")
}

func revision(path string) string {
	cmd := exec.Command("git", "-C", path, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
