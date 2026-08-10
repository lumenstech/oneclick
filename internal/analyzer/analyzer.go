package analyzer

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Plan struct {
	Version      int          `json:"version"`
	PlanHash     string       `json:"plan_hash"`
	App          App          `json:"app"`
	Source       Source       `json:"source"`
	Runtime      Runtime      `json:"runtime"`
	Requirements Requirements `json:"requirements"`
	Services     []Service    `json:"services"`
	Ports        []int        `json:"ports"`
	Secrets      []string     `json:"secrets"`
	Health       Health       `json:"health"`
	Deployment   Deployment   `json:"deployment"`
	Evidence     []string     `json:"evidence"`
	Warnings     []string     `json:"warnings"`
}

type App struct {
	Name string `json:"name"`
}

type Source struct {
	Type     string `json:"type"`
	Ref      string `json:"ref"`
	Revision string `json:"revision,omitempty"`
	Dirty    bool   `json:"dirty,omitempty"`
}

type Runtime struct {
	Primary    string   `json:"primary"`
	Detected   []string `json:"detected"`
	Executor   string   `json:"executor"`
	Deployable bool     `json:"deployable"`
}

type Requirements struct {
	CPUCoresMin int  `json:"cpu_cores_min"`
	MemoryMBMin int  `json:"memory_mb_min"`
	DiskMBMin   int  `json:"disk_mb_min"`
	GPURequired bool `json:"gpu_required"`
}

type Service struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type Health struct {
	Path string `json:"path,omitempty"`
	Port int    `json:"port,omitempty"`
}

type Deployment struct {
	RollbackSupported bool `json:"rollback_supported"`
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var exposeRE = regexp.MustCompile(`(?i)^\s*EXPOSE\s+([0-9]+)`)

func Analyze(root, sourceRef, sourceType, revision string, dirty bool) (Plan, error) {
	info, err := os.Stat(root)
	if err != nil {
		return Plan{}, err
	}
	if !info.IsDir() {
		return Plan{}, fmt.Errorf("source is not a directory: %s", root)
	}

	plan := Plan{
		Version:      1,
		App:          App{Name: sourceName(root, sourceRef, sourceType)},
		Source:       Source{Type: sourceType, Ref: sourceRef, Revision: revision, Dirty: dirty},
		Requirements: Requirements{CPUCoresMin: 2, MemoryMBMin: 2048, DiskMBMin: 2048},
		Deployment:   Deployment{RollbackSupported: false},
	}

	fileExists := func(names ...string) (string, bool) {
		for _, name := range names {
			p := filepath.Join(root, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return name, true
			}
		}
		return "", false
	}

	detected := map[string]bool{}
	add := func(runtime, evidence string) {
		detected[runtime] = true
		if evidence != "" {
			plan.Evidence = append(plan.Evidence, evidence)
		}
	}

	if name, ok := fileExists("compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"); ok {
		add("docker-compose", name)
		plan.Runtime.Executor = "docker-compose"
		plan.Runtime.Deployable = true
		plan.Requirements.MemoryMBMin = max(plan.Requirements.MemoryMBMin, 4096)
	}
	if _, ok := fileExists("Dockerfile"); ok {
		add("docker", "Dockerfile")
		if plan.Runtime.Executor == "" {
			plan.Runtime.Executor = "docker"
			plan.Runtime.Deployable = true
		}
	}
	if _, ok := fileExists("package.json"); ok {
		add("node", "package.json")
		plan.Requirements.MemoryMBMin = max(plan.Requirements.MemoryMBMin, 2048)
	}
	if name, ok := fileExists("pnpm-lock.yaml", "package-lock.json", "yarn.lock", "bun.lock", "bun.lockb"); ok {
		plan.Evidence = append(plan.Evidence, name)
	}
	if name, ok := fileExists("pyproject.toml", "requirements.txt", "Pipfile"); ok {
		add("python", name)
		plan.Requirements.MemoryMBMin = max(plan.Requirements.MemoryMBMin, 2048)
	}
	if _, ok := fileExists("go.mod"); ok {
		add("go", "go.mod")
	}
	if _, ok := fileExists("Cargo.toml"); ok {
		add("rust", "Cargo.toml")
		plan.Requirements.MemoryMBMin = max(plan.Requirements.MemoryMBMin, 2048)
	}

	scanSignals(root, &plan, add)

	var runtimes []string
	for runtime := range detected {
		runtimes = append(runtimes, runtime)
	}
	sort.Strings(runtimes)
	plan.Runtime.Detected = runtimes
	plan.Runtime.Primary = choosePrimary(runtimes)
	if plan.Runtime.Executor == "" {
		plan.Runtime.Executor = "unsupported"
		plan.Runtime.Deployable = false
		if len(runtimes) > 0 {
			plan.Warnings = append(plan.Warnings, "runtime detected, but V0.1 has no safe executor for this repository unless Dockerfile or Docker Compose is present")
		} else {
			plan.Warnings = append(plan.Warnings, "no supported runtime markers detected")
		}
	}

	plan.Ports = detectPorts(root)
	plan.Secrets = detectSecretNames(root)
	plan.Services = detectServices(root, runtimes)
	plan.Warnings = append(plan.Warnings, "hardware requirements are V0.1 heuristics derived from repository signals, not benchmarked sizing")
	if dirty {
		plan.Warnings = append(plan.Warnings, "local Git working tree has uncommitted changes; HEAD revision does not bind the analyzed working-tree contents")
	}
	plan.Evidence = uniqueSorted(plan.Evidence)
	plan.Warnings = uniqueSorted(plan.Warnings)
	plan.PlanHash = PlanHash(plan)
	return plan, nil
}

func scanSignals(root string, plan *Plan, add func(string, string)) {
	needles := map[string][]string{
		"ollama": {"ollama", "OLLAMA_HOST", "ollama/ollama"},
		"vllm":   {"vllm", "vllm/vllm-openai"},
		"cuda":   {"nvidia/cuda", "CUDA_VISIBLE_DEVICES", "--gpus", "runtime: nvidia"},
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && shouldSkipDir(d.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if !isScannable(rel) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil || len(b) > 2<<20 {
			return nil
		}
		s := string(b)
		for runtime, ns := range needles {
			for _, n := range ns {
				if strings.Contains(s, n) {
					add(runtime, rel+":"+n)
					if runtime == "cuda" {
						plan.Requirements.GPURequired = true
						plan.Requirements.MemoryMBMin = max(plan.Requirements.MemoryMBMin, 8192)
					}
					if runtime == "vllm" {
						plan.Warnings = append(plan.Warnings, "vLLM detected; accelerator requirements depend on the selected backend and model, so V0.1 only marks GPU required when CUDA/NVIDIA signals are also present")
					}
					if runtime == "ollama" {
						plan.Requirements.MemoryMBMin = max(plan.Requirements.MemoryMBMin, 8192)
						plan.Warnings = append(plan.Warnings, "Ollama detected; GPU acceleration is optional for some models but model-specific RAM/VRAM sizing is not inferred in V0.1")
					}
					break
				}
			}
		}
		return nil
	})
}

func detectPorts(root string) []int {
	ports := map[int]bool{}
	p := filepath.Join(root, "Dockerfile")
	if f, err := os.Open(p); err == nil {
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			m := exposeRE.FindStringSubmatch(s.Text())
			if len(m) == 2 {
				if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n <= 65535 {
					ports[n] = true
				}
			}
		}
	}
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		re := regexp.MustCompile(`(?m)["']?([0-9]{2,5}):([0-9]{2,5})["']?`)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			n, _ := strconv.Atoi(m[1])
			if n > 0 && n <= 65535 {
				ports[n] = true
			}
		}
	}
	var out []int
	for p := range ports {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

func detectSecretNames(root string) []string {
	var names []string
	for _, file := range []string{".env.example", ".env.sample", "example.env"} {
		f, err := os.Open(filepath.Join(root, file))
		if err != nil {
			continue
		}
		s := bufio.NewScanner(f)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key := line
			if i := strings.IndexByte(key, '='); i >= 0 {
				key = key[:i]
			}
			key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
			if envName.MatchString(key) {
				names = append(names, key)
			}
		}
		_ = f.Close()
	}
	return uniqueSorted(names)
}

func detectServices(root string, runtimes []string) []Service {
	var out []Service
	for _, r := range runtimes {
		switch r {
		case "docker-compose":
			out = append(out, Service{Name: "compose-stack", Kind: "container-stack"})
		case "ollama":
			out = append(out, Service{Name: "ollama", Kind: "model-runtime"})
		case "vllm":
			out = append(out, Service{Name: "vllm", Kind: "model-server"})
		}
	}
	if len(out) == 0 {
		out = append(out, Service{Name: sanitizeName(filepath.Base(root)), Kind: choosePrimary(runtimes)})
	}
	return out
}

func choosePrimary(runtimes []string) string {
	priority := []string{"docker-compose", "docker", "vllm", "ollama", "node", "python", "go", "rust", "cuda"}
	set := map[string]bool{}
	for _, r := range runtimes {
		set[r] = true
	}
	for _, r := range priority {
		if set[r] {
			return r
		}
	}
	return "unknown"
}

func isScannable(rel string) bool {
	base := filepath.Base(rel)
	switch base {
	case "Dockerfile", "compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml", "package.json", "pyproject.toml", "requirements.txt", "README.md", ".env.example":
		return true
	}
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".json", ".yaml", ".yml", ".toml", ".md":
		return true
	}
	return false
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".next", "dist", "build", ".venv", "venv", "target":
		return true
	}
	return false
}

func sourceName(root, ref, sourceType string) string {
	if sourceType == "github" {
		clean := strings.TrimSuffix(ref, "/")
		clean = strings.TrimSuffix(clean, ".git")
		if i := strings.LastIndex(clean, "/"); i >= 0 && i+1 < len(clean) {
			return sanitizeName(clean[i+1:])
		}
		if i := strings.LastIndex(clean, ":"); i >= 0 && i+1 < len(clean) {
			return sanitizeName(filepath.Base(clean[i+1:]))
		}
	}
	return sanitizeName(filepath.Base(root))
}

func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "app"
	}
	return out
}

func uniqueSorted(in []string) []string {
	set := map[string]bool{}
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			set[s] = true
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func PlanHash(p Plan) string {
	p.PlanHash = ""
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
