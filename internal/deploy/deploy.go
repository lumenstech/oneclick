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

	env, cleanup, err := isolatedDockerEnv()
	if err != nil {
		return err
	}
	defer cleanup()

	switch plan.Runtime.Executor {
	case "docker-compose":
		project := composeProjectName(plan)
		file, err := validateCompose(root, project, env)
		if err != nil {
			return err
		}
		return runWithEnv(root, env, "docker", "compose", "-p", project, "-f", file, "up", "-d", "--build")
	case "docker":
		name := imageName(plan)
		if err := runWithEnv(root, env, "docker", "build", "-t", name, "."); err != nil {
			return err
		}
		container := "oneclick-" + plan.App.Name
		cmd := exec.Command("docker", "rm", "-f", container)
		cmd.Dir = root
		cmd.Env = env
		_ = cmd.Run()

		args := []string{"run", "-d", "--name", container, "--restart", "unless-stopped", "--security-opt", "no-new-privileges:true"}
		hostPort := opts.Port
		containerPort := 0
		if len(plan.Ports) == 1 {
			containerPort = plan.Ports[0]
		}
		if hostPort == 0 {
			hostPort = containerPort
		}
		if hostPort > 0 && containerPort > 0 {
			args = append(args, "-p", strconv.Itoa(hostPort)+":"+strconv.Itoa(containerPort))
		} else if hostPort > 0 {
			args = append(args, "-p", strconv.Itoa(hostPort)+":"+strconv.Itoa(hostPort))
		}
		args = append(args, name)
		return runWithEnv(root, env, "docker", args...)
	default:
		return fmt.Errorf("unsupported executor: %s", plan.Runtime.Executor)
	}
}

func isolatedDockerEnv() ([]string, func(), error) {
	home, err := os.MkdirTemp("", "oneclick-docker-home-*")
	if err != nil {
		return nil, nil, err
	}
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"COMPOSE_DISABLE_ENV_FILE=1",
		"LANG=C.UTF-8",
	}
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		env = append(env, "TMPDIR="+tmp)
	}
	return env, func() { _ = os.RemoveAll(home) }, nil
}

func runWithEnv(root string, env []string, name string, args ...string) error {
	fmt.Fprintf(os.Stderr, "+ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Env = env
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

func composeProjectName(plan analyzer.Plan) string {
	sum := sha256.Sum256([]byte(plan.Source.Ref))
	suffix := hex.EncodeToString(sum[:])[:8]
	app := strings.Trim(plan.App.Name, "-_")
	if app == "" {
		app = "app"
	}
	if len(app) > 32 {
		app = app[:32]
	}
	return "oneclick-" + app + "-" + suffix
}

func imageName(plan analyzer.Plan) string {
	planHash := plan.PlanHash
	if planHash == "" {
		planHash = analyzer.StablePlanHash(plan)
	}
	sum := sha256.Sum256([]byte(plan.Source.Ref + planHash))
	tag := hex.EncodeToString(sum[:])[:12]
	return filepath.ToSlash("oneclick/" + plan.App.Name + ":" + tag)
}
