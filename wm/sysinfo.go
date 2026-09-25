package wm

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
)

// What the expanded info bar shows about the machine, sampled every
// second while it is shown. Anything a system doesn't tell is left out.

type hostInfo struct {
	hostname string
	model    string // the machine's
	cpuModel string
	cores    int
	memTotal uint64
	kernel   string
	boot     time.Time
}

type sensorReading struct {
	name string
	temp float64 // °C
}

type gpuInfo struct {
	name     string
	util     float64 // percent, -1 when unknown
	memUsed  uint64  // 0 when unknown
	memTotal uint64
	temp     float64 // °C, -1 when unknown
}

type memInfo struct {
	total, used, available, cached, free uint64
	swapTotal, swapUsed                  uint64
}

// ioRate is a disk's or a network interface's throughput, in bytes per
// second: read and written, or received and sent.
type ioRate struct {
	name    string
	in, out float64
}

type sysSnapshot struct {
	host      hostInfo
	cpuTotal  float64
	cpus      []float64
	coreTemps map[int]float64
	cpuTemps  []float64 // the CPU's sensors, when not one per core
	sensors   []sensorReading
	gpus      []gpuInfo
	mem       memInfo
	disks     []ioRate // nil until two samples were taken
	nets      []ioRate
}

// sampler takes snapshots, keeping the counters of the last one to turn
// the next into rates.
type sampler struct {
	host   *hostInfo
	prevAt time.Time
	disk   map[string][2]uint64
	net    map[string][2]uint64
}

func (s *sampler) sample() sysSnapshot {
	if s.host == nil {
		h := collectHost()
		s.host = &h
	}
	now := time.Now()
	snap := sysSnapshot{host: *s.host}

	if p, err := cpu.Percent(0, true); err == nil {
		snap.cpus = p
		for _, v := range p {
			snap.cpuTotal += v / float64(len(p))
		}
	}

	temps := collectSensors()
	snap.coreTemps, snap.cpuTemps, snap.sensors = sortSensors(temps)
	snap.gpus = collectGPUs()

	if vm, err := mem.VirtualMemory(); err == nil {
		snap.mem = memInfo{total: vm.Total, used: vm.Used, available: vm.Available,
			cached: vm.Cached, free: vm.Free, swapTotal: vm.SwapTotal, swapUsed: vm.SwapTotal - vm.SwapFree}
	}
	if sw, err := mem.SwapMemory(); err == nil {
		snap.mem.swapTotal, snap.mem.swapUsed = sw.Total, sw.Used
	}

	disks := map[string][2]uint64{}
	if io, err := disk.IOCounters(); err == nil {
		for name, c := range io {
			disks[name] = [2]uint64{c.ReadBytes, c.WriteBytes}
		}
	}
	nets := map[string][2]uint64{}
	if io, err := net.IOCounters(true); err == nil {
		for _, c := range io {
			// the loopback, and the interfaces that never moved a byte
			if strings.HasPrefix(c.Name, "lo") || c.BytesRecv+c.BytesSent == 0 {
				continue
			}
			nets[c.Name] = [2]uint64{c.BytesRecv, c.BytesSent}
		}
	}
	if !s.prevAt.IsZero() {
		dt := now.Sub(s.prevAt).Seconds()
		snap.disks = rates(s.disk, disks, dt)
		snap.nets = rates(s.net, nets, dt)
	}
	s.prevAt, s.disk, s.net = now, disks, nets
	return snap
}

// rates turns two samples of counters, dt seconds apart, into rates, by
// name, for the names in both.
func rates(prev, cur map[string][2]uint64, dt float64) []ioRate {
	if dt <= 0 {
		return nil
	}
	out := []ioRate{}
	for name, c := range cur {
		p, ok := prev[name]
		// counters going back were reset
		if !ok || c[0] < p[0] || c[1] < p[1] {
			continue
		}
		out = append(out, ioRate{name: name, in: float64(c[0]-p[0]) / dt, out: float64(c[1]-p[1]) / dt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func collectHost() hostInfo {
	var h hostInfo
	h.cores, _ = cpu.Counts(true)
	if info, err := cpu.Info(); err == nil && len(info) > 0 {
		h.cpuModel = strings.TrimSpace(info[0].ModelName)
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		h.memTotal = vm.Total
	}
	if info, err := host.Info(); err == nil {
		h.hostname, h.kernel = info.Hostname, info.KernelVersion
		h.boot = time.Unix(int64(info.BootTime), 0)
	}
	h.model = machineModel()
	return h
}

// machineModel names the machine, as its system does.
func machineModel() string {
	sysctl := func(names ...string) string {
		out, err := exec.Command("sysctl", append([]string{"-n"}, names...)...).Output()
		if err != nil {
			return ""
		}
		return strings.Join(strings.Fields(string(out)), " ")
	}
	switch runtime.GOOS {
	case "darwin":
		return sysctl("hw.model")
	case "openbsd":
		// hw.model is the CPU's there
		return sysctl("hw.vendor", "hw.product")
	case "linux":
		var parts []string
		for _, f := range []string{"sys_vendor", "product_name"} {
			if b, err := os.ReadFile("/sys/class/dmi/id/" + f); err == nil {
				parts = append(parts, strings.TrimSpace(string(b)))
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// collectSensors reads the temperature sensors.
func collectSensors() []sensorReading {
	var out []sensorReading
	if runtime.GOOS == "openbsd" {
		// which gopsutil doesn't read there
		if b, err := exec.Command("sysctl", "hw.sensors").Output(); err == nil {
			out = parseOpenBSDSensors(strings.NewReader(string(b)))
		}
		return out
	}
	temps, _ := sensors.SensorsTemperatures()
	for _, t := range temps {
		if t.Temperature > 0 && t.Temperature < 150 {
			out = append(out, sensorReading{name: t.SensorKey, temp: t.Temperature})
		}
	}
	return out
}

var openbsdTemp = regexp.MustCompile(`^hw\.sensors\.([^=]+)=([0-9.]+) degC`)

// parseOpenBSDSensors reads the temperatures in the output of sysctl
// hw.sensors: hw.sensors.cpu0.temp0=46.00 degC.
func parseOpenBSDSensors(r io.Reader) []sensorReading {
	var out []sensorReading
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		m := openbsdTemp.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		if t, err := strconv.ParseFloat(m[2], 64); err == nil {
			out = append(out, sensorReading{name: m[1], temp: t})
		}
	}
	return out
}

var (
	// Linux's coretemp_core_3_input, OpenBSD's cpu3.temp0
	coreSensor = regexp.MustCompile(`(?i)(?:core_?|^cpu)(\d+)(?:_input|\.temp\d+)?$`)
	// the CPU's other sensors: Apple's die sensors, AMD's, packages
	cpuSensor = regexp.MustCompile(`(?i)tdie|k10temp|tctl|package|coretemp|cpu`)
)

// sortSensors sorts temperatures into those of cores, when the sensors
// say which, those of the CPU otherwise, and the others, hottest first.
func sortSensors(temps []sensorReading) (cores map[int]float64, cpuTemps []float64, others []sensorReading) {
	cores = map[int]float64{}
	for _, t := range temps {
		switch {
		case coreSensor.MatchString(t.name):
			n, _ := strconv.Atoi(coreSensor.FindStringSubmatch(t.name)[1])
			cores[n] = max(cores[n], t.temp)
		case cpuSensor.MatchString(t.name):
			cpuTemps = append(cpuTemps, t.temp)
		default:
			others = append(others, t)
		}
	}
	sort.Float64s(cpuTemps)
	sort.Slice(others, func(i, j int) bool { return others[i].temp > others[j].temp })
	return cores, cpuTemps, others
}
