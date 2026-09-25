package wm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"
)

func TestRates(t *testing.T) {
	prev := map[string][2]uint64{"disk0": {1000, 2000}, "gone": {1, 1}, "reset": {500, 500}}
	cur := map[string][2]uint64{"disk0": {3000, 2500}, "new": {9, 9}, "reset": {10, 10}}
	got := rates(prev, cur, 2)
	if len(got) != 1 || got[0].name != "disk0" || got[0].in != 1000 || got[0].out != 250 {
		t.Fatalf("rates = %+v, want disk0 at 1000 and 250 B/s only", got)
	}
	if rates(prev, cur, 0) != nil {
		t.Fatalf("rates over no time")
	}

	// all of them, idle or not, sorted
	names := ioNames(map[string][2]uint64{"utun2": {1, 1}, "en0": {5, 5}, "awdl0": {0, 3}})
	if strings.Join(names, " ") != "awdl0 en0 utun2" {
		t.Fatalf("ioNames = %v", names)
	}
	if r, ok := rateOf(got, "disk0"); !ok || r.in != 1000 {
		t.Fatalf("rateOf disk0 = %+v, %v", r, ok)
	}
	if _, ok := rateOf(got, "new"); ok {
		t.Fatalf("a rate for a disk sampled once")
	}
}

func TestHistories(t *testing.T) {
	var s series
	for i := range historyLen + 10 {
		s.push(float64(i))
	}
	if len(s.vals) != historyLen || s.vals[0] != 10 {
		t.Fatalf("series kept %d values from %v", len(s.vals), s.vals[0])
	}
	if last := s.last(3); len(last) != 3 || last[2] != historyLen+9 {
		t.Fatalf("last(3) = %v", last)
	}
	var none *series
	if none.last(5) != nil {
		t.Fatalf("last on no history")
	}

	// against a top
	cols, top := graphColumns([]float64{0, 50, 100, 200}, 8, 10, 2, 100)
	if top != 100 || len(cols) != 4 || cols[1] != 5 || cols[2] != 10 || cols[3] != 10 {
		t.Fatalf("graphColumns against 100 = %v, %v", cols, top)
	}
	// against the peak, only what fits
	cols, top = graphColumns([]float64{9, 1, 2, 4}, 4, 8, 2, 0)
	if top != 4 || len(cols) != 2 || cols[0] != 4 || cols[1] != 8 {
		t.Fatalf("graphColumns against the peak = %v, %v", cols, top)
	}
	if cols, top := graphColumns([]float64{0, 0}, 10, 10, 2, 0); top != 0 || cols[0] != 0 {
		t.Fatalf("graphColumns of zeros = %v, %v", cols, top)
	}

	h := histories{}
	h.record(sysSnapshot{cpuTotal: 12, cpus: []float64{1, 2}, mem: memInfo{total: 100, used: 25},
		nets: []ioRate{{"en0", 5, 6}}, allSensors: []sensorReading{{"cpu0.temp0", 40}}})
	for key, want := range map[string]float64{"cpu": 12, "cpu1": 2, "mem": 25, "net:en0:out": 6, "temp:cpu0.temp0": 40} {
		if v := h[key].last(1); len(v) != 1 || v[0] != want {
			t.Errorf("history %s = %v, want %v", key, v, want)
		}
	}
}

func TestKeepFilesystem(t *testing.T) {
	for _, tc := range []struct {
		mount, fstype string
		want          bool
	}{
		{"/", "ffs", true}, {"/home", "ffs", true}, {"/System/Volumes/Data", "apfs", true},
		{"/System/Volumes/VM", "apfs", false}, {"/dev", "devfs", false}, {"/proc", "proc", false},
		{"/sys/fs/cgroup", "cgroup2", false},
	} {
		if got := keepFilesystem(tc.mount, tc.fstype); got != tc.want {
			t.Errorf("keepFilesystem(%q, %q) = %v", tc.mount, tc.fstype, got)
		}
	}
}

func TestParseOpenBSDSensors(t *testing.T) {
	out := `hw.sensors.cpu0.temp0=46.00 degC
hw.sensors.cpu1.temp0=48.50 degC
hw.sensors.acpitz0.temp0=40.00 degC (zone temperature)
hw.sensors.acpibat0.volt0=11.10 VDC (voltage)
hw.sensors.softraid0.drive0=online (sd1), OK
`
	got := parseOpenBSDSensors(strings.NewReader(out))
	if len(got) != 3 || got[0] != (sensorReading{"cpu0.temp0", 46}) || got[2] != (sensorReading{"acpitz0.temp0", 40}) {
		t.Fatalf("parsed %+v", got)
	}

	cores, cpuTemps, others := sortSensors(got)
	if cores[0] != 46 || cores[1] != 48.5 || len(cpuTemps) != 0 || len(others) != 1 {
		t.Fatalf("sorted into cores %v, cpu %v, others %v", cores, cpuTemps, others)
	}
}

func TestSortSensors(t *testing.T) {
	cores, cpuTemps, others := sortSensors([]sensorReading{
		{"coretemp_core_0", 50}, {"coretemp_core_1", 52}, {"coretemp_packageid0", 55},
		{"PMU tdie1", 60}, {"nvme_composite", 38}, {"acpitz", 45},
	})
	if len(cores) != 2 || cores[1] != 52 {
		t.Fatalf("cores %v", cores)
	}
	if len(cpuTemps) != 2 || cpuTemps[0] != 55 || cpuTemps[1] != 60 {
		t.Fatalf("cpu temperatures %v", cpuTemps)
	}
	if len(others) != 2 || others[0].name != "acpitz" {
		t.Fatalf("others %v, want the hottest first", others)
	}
}

func TestParseNvidiaSMI(t *testing.T) {
	got := parseNvidiaSMI("NVIDIA GeForce RTX 4090, 37, 2048, 24564, 61\nbroken line\n")
	if len(got) != 1 {
		t.Fatalf("parsed %+v", got)
	}
	g := got[0]
	if g.name != "NVIDIA GeForce RTX 4090" || g.util != 37 || g.memUsed != 2048<<20 || g.memTotal != 24564<<20 || g.temp != 61 {
		t.Fatalf("parsed %+v", g)
	}
}

func TestParseIOAccelerator(t *testing.T) {
	out := `+-o AGXAcceleratorG16X  <class AGXAcceleratorG16X, id 0x100000abc>
    {
      "model" = "Apple M4 Pro"
      "PerformanceStatistics" = {"In use system memory"=1330561024,"Device Utilization %"=26,"Renderer Utilization %"=26}
    }
`
	got := parseIOAccelerator(out)
	if len(got) != 1 || got[0].name != "Apple M4 Pro" || got[0].util != 26 || got[0].memUsed != 1330561024 {
		t.Fatalf("parsed %+v", got)
	}
}

func TestAMDGPUs(t *testing.T) {
	drm := t.TempDir()
	dev := filepath.Join(drm, "card1", "device")
	if err := os.MkdirAll(dev, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]string{"gpu_busy_percent": "42\n", "mem_info_vram_used": "1048576\n", "mem_info_vram_total": "8589934592\n"} {
		if err := os.WriteFile(filepath.Join(dev, name), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := amdGPUs(drm)
	if len(got) != 1 || got[0].name != "card1 (amdgpu)" || got[0].util != 42 || got[0].memUsed != 1<<20 || got[0].memTotal != 8<<30 {
		t.Fatalf("read %+v", got)
	}
}

func TestPanelFormatting(t *testing.T) {
	if h := panelHeight(800, 14); h > 480 || h < 22*panelLineH() {
		t.Fatalf("panel height %d for 14 cores on 800 pixels", h)
	}
	if h := panelHeight(800, 128); h != 480 {
		t.Fatalf("panel height %d for 128 cores, want 60%% of the screen", h)
	}
	if meterColor(10) != draculaGreen || meterColor(60) != draculaYellow || meterColor(95) != draculaRed {
		t.Fatalf("meter colors")
	}
	if got := formatUptime(2*24*time.Hour + 18*time.Hour + 41*time.Minute + 20*time.Second); got != "2d 18h 41m" {
		t.Fatalf("formatUptime = %q", got)
	}
	if got := formatUptime(3*time.Hour + 5*time.Minute); got != "3h 5m" {
		t.Fatalf("formatUptime = %q", got)
	}
}

func TestSample(t *testing.T) {
	var s sampler
	first := s.sample()
	if len(first.cpus) == 0 || first.host.cores == 0 || first.mem.total == 0 {
		t.Fatalf("first sample %+v", first)
	}
	if first.disks != nil || first.nets != nil {
		t.Fatalf("rates from a single sample")
	}
	time.Sleep(50 * time.Millisecond)
	if second := s.sample(); second.nets == nil || second.disks == nil {
		t.Fatalf("no rates from the second sample")
	}
}

func TestPanelToggle(t *testing.T) {
	wm := newTestManager(t)
	drainEvents(t, wm)
	s := wm.GetActiveScreen()
	viewable := func() bool {
		attr, err := xproto.GetWindowAttributes(wm.Conn(), s.panel.window).Reply()
		if err != nil {
			t.Fatal(err)
		}
		return attr.MapState == xproto.MapStateViewable
	}

	// Mod+s, as the binding does
	if err := s.togglePanel(); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	if !viewable() {
		t.Fatalf("panel not shown")
	}
	// drawn: the headers are in the accent color
	img, err := xproto.GetImage(wm.Conn(), xproto.ImageFormatZPixmap, xproto.Drawable(s.panel.window),
		0, 0, s.Geometry().W, uint16(s.panel.h), ^uint32(0)).Reply()
	if err != nil {
		t.Fatal(err)
	}
	accent := 0
	for i := 0; i+3 < len(img.Data); i += 4 {
		if uint32(img.Data[i+2])<<16|uint32(img.Data[i+1])<<8|uint32(img.Data[i]) == colorAccent {
			accent++
		}
	}
	if accent < 1000 {
		t.Fatalf("%d pixels of the accent color in the panel", accent)
	}

	// a click on the bar hides it, another shows it
	for _, want := range []bool{false, true} {
		wm.handleEvent(xproto.ButtonPressEvent{Event: wm.GetActiveWorkspace().InfoBarWindow, Detail: 1})
		drainEvents(t, wm)
		if viewable() != want {
			t.Fatalf("panel shown=%v after a click on the bar, want %v", !want, want)
		}
	}
}

func TestPanelViews(t *testing.T) {
	wm := newTestManager(t)
	s := wm.GetActiveScreen()
	if err := s.togglePanel(); err != nil {
		t.Fatal(err)
	}
	p := s.panel
	for range 3 {
		time.Sleep(50 * time.Millisecond)
		p.refresh()
	}
	drainEvents(t, wm)

	// keys switch the views
	for _, step := range []struct {
		sym   xproto.Keysym
		state uint16
		want  string
	}{
		{XK_Tab, 0, "CPU"}, {XK_Left, 0, "Summary"}, {'4', 0, "Disk"},
		{XK_Tab, xproto.ModMaskShift, "Memory"}, {XK_Right, 0, "Disk"},
	} {
		press(t, wm, step.sym, step.state)
		if got := panelViews[p.view]; got != step.want {
			t.Fatalf("view %s after key 0x%x, want %s", got, uint32(step.sym), step.want)
		}
	}

	// so does a click on a view's name
	tab := p.tabs[5]
	wm.handleEvent(xproto.ButtonPressEvent{Event: p.window, Detail: 1, EventX: int16((tab[0] + tab[1]) / 2), EventY: int16(panelPad)})
	if panelViews[p.view] != "Sensors" {
		t.Fatalf("view %s after clicking Sensors", panelViews[p.view])
	}

	// every view draws, without X errors
	for i := range panelViews {
		p.view = i
		p.draw()
		drainEvents(t, wm)
	}

	press(t, wm, XK_Escape, 0)
	if p.shown {
		t.Fatalf("Escape didn't close the panel")
	}
}
