package deploy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var composeKeyRE = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*:`)
var composeInterpolationRE = regexp.MustCompile(`\$\{[^}]+\}`)

var composeRawForbiddenKeys = map[string]string{
	"include":         "remote or transitive Compose includes are not supported",
	"extends":         "Compose extends can import unreviewed configuration",
	"env_file":        "Compose env_file can read host files",
	"label_file":      "Compose label_file can read host files",
	"secrets":         "Compose secrets are not supported in V0.1 because file-backed secrets can read host files",
	"configs":         "Compose configs are not supported in V0.1 because file-backed configs can read host files",
	"provider":        "Compose providers can execute host binaries",
	"credential_spec": "credential_spec can reference host files",
	"use_api_socket":  "use_api_socket exposes the container engine API socket",
}

func validateCompose(root, project string, env []string) (string, error) {
	path, err := composeFile(root)
	if err != nil {
		return "", err
	}
	if err := scanComposeSource(path); err != nil {
		return "", err
	}
	file := filepath.Base(path)
	out, err := composeConfig(root, file, project, env)
	if err != nil {
		return "", err
	}
	var model any
	if err := json.Unmarshal(out, &model); err != nil {
		return "", fmt.Errorf("docker compose config returned invalid JSON: %w", err)
	}
	if err := validateComposeModel(model, "$", project); err != nil {
		return "", err
	}
	return file, nil
}

func composeFile(root string) (string, error) {
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"} {
		path := filepath.Join(root, name)
		if st, err := os.Lstat(path); err == nil && !st.IsDir() {
			if st.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("Compose file must not be a symlink: %s", name)
			}
			return path, nil
		}
	}
	return "", fmt.Errorf("compose executor selected but no Compose file exists")
}

func scanComposeSource(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := stripComposeComment(scanner.Text())
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if composeInterpolationRE.MatchString(trimmed) {
			return fmt.Errorf("unsafe Compose configuration at line %d: environment interpolation is not supported in V0.1", lineNo)
		}
		keyLine := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		if m := composeKeyRE.FindStringSubmatch(keyLine); len(m) == 2 {
			if reason, blocked := composeRawForbiddenKeys[strings.ToLower(m[1])]; blocked {
				return fmt.Errorf("unsafe Compose configuration at line %d: %s", lineNo, reason)
			}
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "/var/run/docker.sock") || strings.Contains(lower, "/run/docker.sock") {
			return fmt.Errorf("unsafe Compose configuration at line %d: Docker socket access is not permitted", lineNo)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func stripComposeComment(line string) string {
	inSingle, inDouble := false, false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

func composeConfig(root, file, project string, env []string) ([]byte, error) {
	args := []string{"compose", "-p", project, "-f", file, "config", "--format", "json", "--no-env-resolution", "--no-interpolate", "--no-path-resolution"}
	cmd := exec.Command("docker", args...)
	cmd.Dir = root
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker compose config safety check failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func validateComposeModel(v any, path, project string) error {
	switch x := v.(type) {
	case map[string]any:
		if typ, _ := x["type"].(string); strings.EqualFold(typ, "bind") {
			return fmt.Errorf("unsafe Compose configuration at %s: host bind mounts are not permitted", path)
		}
		for k, child := range x {
			lk := strings.ToLower(k)
			childPath := path + "." + k
			switch lk {
			case "privileged", "use_api_socket":
				if b, ok := child.(bool); ok && b {
					return fmt.Errorf("unsafe Compose configuration at %s: %s is not permitted", childPath, k)
				}
			case "cap_add", "security_opt", "volumes_from", "extra_hosts", "sysctls", "storage_opt":
				if nonEmpty(child) {
					return fmt.Errorf("unsafe Compose configuration at %s: %s is not permitted", childPath, k)
				}
			case "provider", "env_file", "label_file", "include", "extends", "credential_spec":
				if nonEmpty(child) {
					return fmt.Errorf("unsafe Compose configuration at %s: %s is not permitted", childPath, k)
				}
			case "network_mode", "pid", "ipc", "uts", "userns_mode", "cgroup", "runtime":
				if s, _ := child.(string); strings.TrimSpace(s) != "" {
					return fmt.Errorf("unsafe Compose configuration at %s: explicit %s is not permitted", childPath, k)
				}
			case "driver_opts":
				if nonEmpty(child) {
					return fmt.Errorf("unsafe Compose configuration at %s: volume/network driver options are not permitted", childPath)
				}
			case "entitlements", "ssh", "additional_contexts":
				if nonEmpty(child) {
					return fmt.Errorf("unsafe Compose configuration at %s: build %s is not permitted", childPath, k)
				}
			case "network":
				if s, _ := child.(string); strings.EqualFold(strings.TrimSpace(s), "host") {
					return fmt.Errorf("unsafe Compose configuration at %s: host build networking is not permitted", childPath)
				}
			case "context", "dockerfile":
				if s, ok := child.(string); ok && !safeComposeRelativePath(s) {
					return fmt.Errorf("unsafe Compose configuration at %s: path must stay inside the repository", childPath)
				}
			case "secrets", "configs":
				if nonEmpty(child) {
					return fmt.Errorf("unsafe Compose configuration at %s: file-backed secret/config surfaces are not permitted in V0.1", childPath)
				}
			case "models":
				if path == "$" && nonEmpty(child) {
					return fmt.Errorf("unsafe Compose configuration at %s: Compose models are not supported in V0.1", childPath)
				}
			case "devices":
				if nonEmpty(child) && !strings.Contains(path, ".deploy.resources.reservations") {
					return fmt.Errorf("unsafe Compose configuration at %s: direct host device access is not permitted", childPath)
				}
			case "external":
				if b, ok := child.(bool); ok && b && (strings.HasPrefix(path, "$.volumes.") || strings.HasPrefix(path, "$.networks.")) {
					return fmt.Errorf("unsafe Compose configuration at %s: external volumes/networks are not permitted", childPath)
				}
			case "name":
				if s, ok := child.(string); ok && s != "" && (strings.HasPrefix(path, "$.volumes.") || strings.HasPrefix(path, "$.networks.")) {
					if !strings.HasPrefix(s, project+"_") {
						return fmt.Errorf("unsafe Compose configuration at %s: resource name escapes the OneClick project namespace", childPath)
					}
				}
			}
			if err := validateComposeModel(child, childPath, project); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range x {
			if err := validateComposeModel(child, fmt.Sprintf("%s[%d]", path, i), project); err != nil {
				return err
			}
		}
	}
	return nil
}

func safeComposeRelativePath(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if strings.Contains(s, "://") || filepath.IsAbs(s) || strings.HasPrefix(s, "~") {
		return false
	}
	clean := filepath.Clean(s)
	return clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func nonEmpty(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x) != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	case bool:
		return x
	default:
		return true
	}
}
