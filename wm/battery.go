package wm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The battery, on laptops: its charge, the time it has left on battery or
// to be full, and whether the machine is plugged in. Read from apm(8) on
// OpenBSD, /sys on Linux, pmset(1) on macOS and sysctl(8) on FreeBSD.

type batteryState int

const (
	batteryDischarging batteryState = iota
	batteryCharging
	batteryFull // plugged in, not charging
)

type batteryInfo struct {
	percent float64
	state   batteryState
	left    time.Duration // to empty or to full, 0 when unknown
}

// batteries change slowly, and some systems are read by running commands
const batteryTTL = 10 * time.Second

var batteryCache struct {
	at      time.Time
	info    batteryInfo
	present bool
}

// readBattery returns the battery's state, and false when there is none.
func readBattery() (batteryInfo, bool) {
	if time.Since(batteryCache.at) < batteryTTL {
		return batteryCache.info, batteryCache.present
	}
	var info batteryInfo
	present := false
	switch runtime.GOOS {
	case "openbsd":
		info, present = openbsdBattery()
	case "linux":
		info, present = linuxBattery("/sys/class/power_supply")
	case "darwin":
		if out, err := exec.Command("pmset", "-g", "batt").Output(); err == nil {
			info, present = parsePmset(string(out))
		}
	case "freebsd":
		info, present = freebsdBattery()
	}
	batteryCache.at, batteryCache.info, batteryCache.present = time.Now(), info, present
	return info, present
}

// formatBattery shows a battery's state, as BAT: 85% (3h45 left).
func formatBattery(b batteryInfo) string {
	s := fmt.Sprintf("BAT: %.0f%%", b.percent)
	switch b.state {
	case batteryCharging:
		s += " charging"
		if b.left > 0 {
			s += fmt.Sprintf(" (%s to full)", formatHM(b.left))
		}
	case batteryFull:
		s += " plugged"
	default:
		if b.left > 0 {
			s += fmt.Sprintf(" (%s left)", formatHM(b.left))
		}
	}
	return s
}

// batteryAlert tells whether the battery is running low, on battery.
func batteryAlert(b batteryInfo) bool {
	return b.state == batteryDischarging && b.percent <= alertBatteryPercent
}

func formatHM(d time.Duration) string {
	d = d.Round(time.Minute)
	return fmt.Sprintf("%dh%02d", int(d.Hours()), int(d.Minutes())%60)
}

func openbsdBattery() (batteryInfo, bool) {
	apm := func(flag string) string {
		out, err := exec.Command("apm", flag).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	return parseAPM(apm("-b"), apm("-l"), apm("-m"), apm("-a"))
}

// parseAPM reads apm's -b, -l, -m and -a outputs: the battery's state, its
// charge in percent, its minutes left, and the AC's state.
func parseAPM(state, life, minutes, ac string) (batteryInfo, bool) {
	// 4 is no battery, 255 unknown
	if state == "" || state == "4" || state == "255" {
		return batteryInfo{}, false
	}
	percent, err := strconv.ParseFloat(life, 64)
	if err != nil || percent < 0 {
		return batteryInfo{}, false
	}
	b := batteryInfo{percent: percent}
	switch {
	case state == "3":
		b.state = batteryCharging
	case ac == "1":
		b.state = batteryFull
	}
	// the battery's life left, not a time to be full
	if m, err := strconv.Atoi(minutes); err == nil && m > 0 && b.state == batteryDischarging {
		b.left = time.Duration(m) * time.Minute
	}
	return b, true
}

// linuxBattery reads the first battery under power_supply, and whether
// the machine is plugged in.
func linuxBattery(dir string) (batteryInfo, bool) {
	read := func(dev, name string) string {
		b, _ := os.ReadFile(filepath.Join(dev, name))
		return strings.TrimSpace(string(b))
	}
	num := func(dev, name string) float64 {
		v, _ := strconv.ParseFloat(read(dev, name), 64)
		return v
	}
	devs, _ := filepath.Glob(filepath.Join(dir, "*"))
	plugged := false
	for _, dev := range devs {
		if read(dev, "type") == "Mains" && read(dev, "online") == "1" {
			plugged = true
		}
	}
	for _, dev := range devs {
		if read(dev, "type") != "Battery" || read(dev, "present") == "0" {
			continue
		}
		b := batteryInfo{percent: num(dev, "capacity")}
		status := read(dev, "status")
		switch {
		case status == "Charging":
			b.state = batteryCharging
		case status == "Full" || status == "Not charging" || (plugged && status != "Discharging"):
			b.state = batteryFull
		}

		// energy in µWh and power in µW, or charge in µAh and current in µA
		now, full, rate := num(dev, "energy_now"), num(dev, "energy_full"), num(dev, "power_now")
		if now == 0 {
			now, full, rate = num(dev, "charge_now"), num(dev, "charge_full"), num(dev, "current_now")
		}
		if rate > 0 {
			switch b.state {
			case batteryDischarging:
				b.left = time.Duration(now / rate * float64(time.Hour))
			case batteryCharging:
				if full > now {
					b.left = time.Duration((full - now) / rate * float64(time.Hour))
				}
			}
		}
		return b, true
	}
	return batteryInfo{}, false
}

var (
	pmsetPercent = regexp.MustCompile(`(\d+)%;\s*([^;]+);\s*(?:(\d+):(\d+) remaining)?`)
)

// parsePmset reads pmset -g batt: "InternalBattery-0 (id=...)	85%;
// discharging; 3:45 remaining present: true".
func parsePmset(out string) (batteryInfo, bool) {
	m := pmsetPercent.FindStringSubmatch(out)
	if m == nil {
		return batteryInfo{}, false
	}
	percent, _ := strconv.ParseFloat(m[1], 64)
	b := batteryInfo{percent: percent}
	switch strings.TrimSpace(m[2]) {
	case "charging":
		b.state = batteryCharging
	case "discharging":
		b.state = batteryDischarging
	default: // charged, AC attached, finishing charge
		b.state = batteryFull
	}
	if m[3] != "" {
		h, _ := strconv.Atoi(m[3])
		min, _ := strconv.Atoi(m[4])
		b.left = time.Duration(h)*time.Hour + time.Duration(min)*time.Minute
	}
	return b, true
}

func freebsdBattery() (batteryInfo, bool) {
	sysctl := func(name string) string {
		out, err := exec.Command("sysctl", "-n", name).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	return parseFreeBSDBattery(sysctl("hw.acpi.battery.life"), sysctl("hw.acpi.battery.time"),
		sysctl("hw.acpi.battery.state"), sysctl("hw.acpi.acline"))
}

// parseFreeBSDBattery reads hw.acpi.battery.life, .time in minutes, .state,
// a bit mask where 1 is discharging and 2 charging, and hw.acpi.acline.
func parseFreeBSDBattery(life, minutes, state, acline string) (batteryInfo, bool) {
	percent, err := strconv.ParseFloat(life, 64)
	if err != nil || percent < 0 {
		return batteryInfo{}, false
	}
	b := batteryInfo{percent: percent}
	s, _ := strconv.Atoi(state)
	switch {
	case s&2 != 0:
		b.state = batteryCharging
	case s&1 == 0 && acline == "1":
		b.state = batteryFull
	}
	if m, err := strconv.Atoi(minutes); err == nil && m > 0 {
		b.left = time.Duration(m) * time.Minute
	}
	return b, true
}
