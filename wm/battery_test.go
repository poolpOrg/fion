package wm

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseAPM(t *testing.T) {
	for _, tc := range []struct {
		state, life, minutes, ac string
		want                     batteryInfo
		present                  bool
	}{
		{"0", "85", "225", "0", batteryInfo{85, batteryDischarging, 225 * time.Minute}, true},
		{"3", "60", "400", "1", batteryInfo{60, batteryCharging, 0}, true}, // no time to full from apm
		{"0", "100", "unknown", "1", batteryInfo{100, batteryFull, 0}, true},
		{"4", "-1", "unknown", "1", batteryInfo{}, false}, // no battery
		{"", "", "", "", batteryInfo{}, false},            // no apm
	} {
		got, ok := parseAPM(tc.state, tc.life, tc.minutes, tc.ac)
		if ok != tc.present || got != tc.want {
			t.Errorf("parseAPM(%q, %q, %q, %q) = %+v, %v, want %+v, %v",
				tc.state, tc.life, tc.minutes, tc.ac, got, ok, tc.want, tc.present)
		}
	}
}

func TestLinuxBattery(t *testing.T) {
	dir := t.TempDir()
	write := func(dev string, files map[string]string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, dev), 0o755); err != nil {
			t.Fatal(err)
		}
		for name, v := range files {
			if err := os.WriteFile(filepath.Join(dir, dev, name), []byte(v+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	write("AC", map[string]string{"type": "Mains", "online": "0"})
	// 30 Wh left at 10 W: 3 hours
	write("BAT0", map[string]string{"type": "Battery", "present": "1", "capacity": "50", "status": "Discharging",
		"energy_now": "30000000", "energy_full": "60000000", "power_now": "10000000"})
	got, ok := linuxBattery(dir)
	if !ok || got != (batteryInfo{50, batteryDischarging, 3 * time.Hour}) {
		t.Fatalf("discharging: %+v, %v", got, ok)
	}

	// charging at 15 W, 30 Wh to go: 2 hours
	write("AC", map[string]string{"online": "1"})
	write("BAT0", map[string]string{"status": "Charging", "power_now": "15000000"})
	got, ok = linuxBattery(dir)
	if !ok || got != (batteryInfo{50, batteryCharging, 2 * time.Hour}) {
		t.Fatalf("charging: %+v, %v", got, ok)
	}

	if _, ok := linuxBattery(t.TempDir()); ok {
		t.Fatalf("a battery found where there is none")
	}
}

func TestParsePmset(t *testing.T) {
	for _, tc := range []struct {
		out     string
		want    batteryInfo
		present bool
	}{
		{"Now drawing from 'Battery Power'\n -InternalBattery-0 (id=4718691)\t85%; discharging; 3:45 remaining present: true\n",
			batteryInfo{85, batteryDischarging, 3*time.Hour + 45*time.Minute}, true},
		{"Now drawing from 'AC Power'\n -InternalBattery-0 (id=4718691)\t62%; charging; 1:10 remaining present: true\n",
			batteryInfo{62, batteryCharging, time.Hour + 10*time.Minute}, true},
		{"Now drawing from 'AC Power'\n -InternalBattery-0 (id=4718691)\t100%; charged; 0:00 remaining present: true\n",
			batteryInfo{100, batteryFull, 0}, true},
		{"Now drawing from 'AC Power'\n", batteryInfo{}, false}, // a Mac mini
	} {
		got, ok := parsePmset(tc.out)
		if ok != tc.present || got != tc.want {
			t.Errorf("parsePmset(%q) = %+v, %v, want %+v, %v", tc.out, got, ok, tc.want, tc.present)
		}
	}
}

func TestParseFreeBSDBattery(t *testing.T) {
	if got, ok := parseFreeBSDBattery("77", "150", "1", "0"); !ok || got != (batteryInfo{77, batteryDischarging, 150 * time.Minute}) {
		t.Fatalf("discharging: %+v, %v", got, ok)
	}
	if got, ok := parseFreeBSDBattery("90", "-1", "2", "1"); !ok || got.state != batteryCharging {
		t.Fatalf("charging: %+v, %v", got, ok)
	}
	if got, ok := parseFreeBSDBattery("100", "-1", "0", "1"); !ok || got.state != batteryFull {
		t.Fatalf("plugged: %+v, %v", got, ok)
	}
	if _, ok := parseFreeBSDBattery("", "", "", ""); ok {
		t.Fatalf("a battery without sysctl")
	}
}

func TestFormatBattery(t *testing.T) {
	for _, tc := range []struct {
		b     batteryInfo
		want  string
		alert bool
	}{
		{batteryInfo{85, batteryDischarging, 3*time.Hour + 45*time.Minute}, "BAT: 85% (3h45 left)", false},
		{batteryInfo{10, batteryDischarging, 20 * time.Minute}, "BAT: 10% (0h20 left)", true},
		{batteryInfo{10, batteryCharging, time.Hour}, "BAT: 10% charging (1h00 to full)", false},
		{batteryInfo{60, batteryCharging, 0}, "BAT: 60% charging", false},
		{batteryInfo{100, batteryFull, 0}, "BAT: 100% plugged", false},
	} {
		if got := formatBattery(tc.b); got != tc.want {
			t.Errorf("formatBattery(%+v) = %q, want %q", tc.b, got, tc.want)
		}
		if got := batteryAlert(tc.b); got != tc.alert {
			t.Errorf("batteryAlert(%+v) = %v, want %v", tc.b, got, tc.alert)
		}
	}
}
