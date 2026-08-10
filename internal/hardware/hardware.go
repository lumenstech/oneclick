package hardware

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/lumenstech/oneclick/internal/analyzer"
)

type Host struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	CPUCores     int    `json:"cpu_cores"`
	MemoryMB     int    `json:"memory_mb"`
	DiskFreeMB   uint64 `json:"disk_free_mb"`
	Docker       bool   `json:"docker_available"`
	NVIDIA       bool   `json:"nvidia_gpu"`
	NVIDIAName   string `json:"nvidia_name,omitempty"`
	NVIDIAVRAMMB int    `json:"nvidia_vram_mb,omitempty"`
}

type Result struct {
	Host     Host     `json:"host"`
	Fits     bool     `json:"fits"`
	Problems []string `json:"problems"`
	Warnings []string `json:"warnings"`
}

func Inspect(path string) Host {
	h := Host{OS: runtime.GOOS, Architecture: runtime.GOARCH, CPUCores: runtime.NumCPU()}
	h.MemoryMB = linuxMemoryMB()
	h.DiskFreeMB = freeDiskMB(path)
	_, err := exec.LookPath("docker")
	h.Docker = err == nil
	if out, err := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits").Output(); err == nil {
		line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			h.NVIDIA = true
			h.NVIDIAName = strings.TrimSpace(parts[0])
			h.NVIDIAVRAMMB, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		}
	}
	return h
}

func Check(plan analyzer.Plan, path string) Result {
	h := Inspect(path)
	r := Result{Host: h, Fits: true}
	if runtime.GOOS != "linux" {
		r.Fits = false
		r.Problems = append(r.Problems, "V0.1 deployment preflight currently supports Linux Docker hosts only")
	}
	if h.CPUCores < plan.Requirements.CPUCoresMin {
		r.Fits = false
		r.Problems = append(r.Problems, fmt.Sprintf("CPU cores: need at least %d, found %d", plan.Requirements.CPUCoresMin, h.CPUCores))
	}
	if h.MemoryMB > 0 && h.MemoryMB < plan.Requirements.MemoryMBMin {
		r.Fits = false
		r.Problems = append(r.Problems, fmt.Sprintf("memory: need at least %d MB, found %d MB", plan.Requirements.MemoryMBMin, h.MemoryMB))
	}
	if h.DiskFreeMB > 0 && h.DiskFreeMB < uint64(plan.Requirements.DiskMBMin) {
		r.Fits = false
		r.Problems = append(r.Problems, fmt.Sprintf("disk: need at least %d MB free, found %d MB", plan.Requirements.DiskMBMin, h.DiskFreeMB))
	}
	if plan.Requirements.GPURequired && !h.NVIDIA {
		r.Fits = false
		r.Problems = append(r.Problems, "NVIDIA GPU required by explicit CUDA/NVIDIA repository signals but nvidia-smi did not report one")
	}
	if plan.Runtime.Deployable && !h.Docker {
		r.Fits = false
		r.Problems = append(r.Problems, "Docker is required for the detected V0.1 executor but is not available")
	}
	if plan.Requirements.GPURequired && h.NVIDIA {
		r.Warnings = append(r.Warnings, "GPU detected, but model-specific VRAM sizing is not inferred in V0.1")
	}
	return r
}

func linuxMemoryMB() int {
	if runtime.GOOS != "linux" {
		return 0
	}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			kb, _ := strconv.Atoi(fields[1])
			return kb / 1024
		}
	}
	return 0
}
