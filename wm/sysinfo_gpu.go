package wm

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// GPUs, which no library tells about everywhere: nvidia-smi for NVIDIA's,
// the kernel's counters for AMD's on Linux, the graphics driver's
// statistics on macOS. Other systems show none.

func collectGPUs() []gpuInfo {
	var gpus []gpuInfo
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		out, err := exec.Command("nvidia-smi",
			"--query-gpu=name,utilization.gpu,memory.used,memory.total,temperature.gpu",
			"--format=csv,noheader,nounits").Output()
		if err == nil {
			gpus = append(gpus, parseNvidiaSMI(string(out))...)
		}
	}
	switch runtime.GOOS {
	case "linux":
		gpus = append(gpus, amdGPUs("/sys/class/drm")...)
	case "darwin":
		if out, err := exec.Command("ioreg", "-r", "-d", "1", "-c", "IOAccelerator").Output(); err == nil {
			gpus = append(gpus, parseIOAccelerator(string(out))...)
		}
	}
	return gpus
}

// parseNvidiaSMI reads nvidia-smi's CSV: name, utilization %, memory used
// and total in MiB, temperature.
func parseNvidiaSMI(out string) []gpuInfo {
	var gpus []gpuInfo
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, ",")
		if len(f) != 5 {
			continue
		}
		for i := range f {
			f[i] = strings.TrimSpace(f[i])
		}
		g := gpuInfo{name: f[0], util: -1, temp: -1}
		if v, err := strconv.ParseFloat(f[1], 64); err == nil {
			g.util = v
		}
		if v, err := strconv.ParseUint(f[2], 10, 64); err == nil {
			g.memUsed = v << 20
		}
		if v, err := strconv.ParseUint(f[3], 10, 64); err == nil {
			g.memTotal = v << 20
		}
		if v, err := strconv.ParseFloat(f[4], 64); err == nil {
			g.temp = v
		}
		gpus = append(gpus, g)
	}
	return gpus
}

// amdGPUs reads the AMD GPUs' busy percentage and memory, which the amdgpu
// driver publishes under drm.
func amdGPUs(drm string) []gpuInfo {
	var gpus []gpuInfo
	paths, _ := filepath.Glob(filepath.Join(drm, "card[0-9]*", "device", "gpu_busy_percent"))
	for _, p := range paths {
		dev := filepath.Dir(p)
		read := func(name string) string {
			b, _ := os.ReadFile(filepath.Join(dev, name))
			return strings.TrimSpace(string(b))
		}
		g := gpuInfo{name: filepath.Base(filepath.Dir(dev)) + " (amdgpu)", util: -1, temp: -1}
		if v, err := strconv.ParseFloat(read("gpu_busy_percent"), 64); err == nil {
			g.util = v
		}
		g.memUsed, _ = strconv.ParseUint(read("mem_info_vram_used"), 10, 64)
		g.memTotal, _ = strconv.ParseUint(read("mem_info_vram_total"), 10, 64)
		gpus = append(gpus, g)
	}
	return gpus
}

var (
	ioregModel = regexp.MustCompile(`"model" = "([^"]+)"`)
	ioregUtil  = regexp.MustCompile(`"Device Utilization %"=(\d+)`)
	ioregMem   = regexp.MustCompile(`"In use system memory"=(\d+)`)
)

// parseIOAccelerator reads the GPUs out of ioreg's IOAccelerator entries:
// their model, and in their PerformanceStatistics, their utilization and
// the memory they use.
func parseIOAccelerator(out string) []gpuInfo {
	var gpus []gpuInfo
	// one entry per "+-o" line
	for _, entry := range strings.Split(out, "+-o")[1:] {
		g := gpuInfo{name: "GPU", util: -1, temp: -1}
		if m := ioregModel.FindStringSubmatch(entry); m != nil {
			g.name = m[1]
		}
		if m := ioregUtil.FindStringSubmatch(entry); m != nil {
			g.util, _ = strconv.ParseFloat(m[1], 64)
		}
		if m := ioregMem.FindStringSubmatch(entry); m != nil {
			g.memUsed, _ = strconv.ParseUint(m[1], 10, 64)
		}
		if g.util >= 0 || g.memUsed > 0 {
			gpus = append(gpus, g)
		}
	}
	return gpus
}
