package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lumenstech/oneclick/internal/analyzer"
)

type Options struct {
	Confirm bool
	Port    int
}

func Run(plan analyzer.Plan, root string, opts Options) error {
	if !opts.Confirm {
		return fmt.Errorf("deployment requires --yes")
	}
	if !plan.Runtime.Deployable {
		return fmt.Errorf("repository has no supported V0.1 executor")
	}
	switch plan.Runtime.Executor {
	case "docker-compose":
		return run(root, "docker", "compose", "up", "-d", "--build")
	case "docker":
		name := imageName(plan)
		if err := run(root, "docker", "build", "-t", name, "."); err != nil {
			return err
		}
		container := "oneclick-" + plan.App.Name
		_ = exec.Command("docker", "rm", "-f", container).Run()
		args := []string{"run", "-d", "--name", container, "--restart", "unless-stopped"}
		port := opts.Port
		if port == 0 && len(plan.Ports) == 1 {
			port = plan.Ports[0]
		}
		if port > 0 {
			args = append(args, "-p", strconv.Itoa(port)+":"+strconv.Itoa(port))
		}
		args = append(args, name)
		return run(root, "docker", args...)
	default:
		return fmt.Errorf("unsupported executor: %s", plan.Runtime.Executor)
	}
}

func run(root, name string, args ...string) error {
	fmt.Fprintf(os.Stderr, "+ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

func imageName(plan analyzer.Plan) string {
	sum := sha256.Sum256([]byte(plan.Source.Ref + analyzer.PlanHash(plan)))
	tag := hex.EncodeToString(sum[:])[:12]
	return filepath.ToSlash("oneclick/" + plan.App.Name + ":" + tag)
}
