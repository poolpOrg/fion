package wm

import (
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"
)

func TestParseOSRelease(t *testing.T) {
	for _, tc := range []struct {
		release, name, version string
	}{
		{"NAME=\"Debian GNU/Linux\"\nVERSION_ID=\"12\"\nID=debian\n", "Debian", "12"},
		{"NAME=\"Fedora Linux\"\nVERSION_ID=40\n", "Fedora", "40"},
		{"NAME=\"Arch Linux\"\nID=arch\n# rolling\n", "Arch", ""},
		{"ID=alpine\nVERSION_ID=3.20.1\n", "alpine", "3.20.1"},
	} {
		name, version := parseOSRelease(strings.NewReader(tc.release))
		if name != tc.name || version != tc.version {
			t.Errorf("parseOSRelease(%q) = %q, %q, want %q, %q", tc.release, name, version, tc.name, tc.version)
		}
	}
}

func TestOSInfo(t *testing.T) {
	o := osInfo{kind: "openbsd", name: "OpenBSD", version: "7.9", machine: "arm64"}
	if got := o.String(); got != "OpenBSD/7.9 (arm64)" {
		t.Fatalf("String() = %q", got)
	}
	if got := (osInfo{name: "Arch", machine: "x86_64"}).String(); got != "Arch (x86_64)" {
		t.Fatalf("String() without a version = %q", got)
	}

	if self := thisOS(); self.name == "" || self.machine == "" {
		t.Fatalf("detected %+v", self)
	}
}

func TestOSIcons(t *testing.T) {
	for _, kind := range []string{"openbsd", "linux", "freebsd", "netbsd", "darwin", "plan9"} {
		ic := osIcon(kind)
		if ic.h > infoBarInnerH() || ic.w > 2*infoBarInnerH() {
			t.Errorf("%s icon is %dx%d, too big for the bar", kind, ic.w, ic.h)
		}
		drawn := 0
		for _, c := range ic.px {
			if c != transparent {
				drawn++
			}
		}
		if drawn < ic.w*ic.h/5 {
			t.Errorf("%s icon has only %d pixels drawn", kind, drawn)
		}
	}
	// the blowfish is yellow
	yellow := 0
	for _, c := range blowfishIcon().px {
		if c == draculaYellow {
			yellow++
		}
	}
	if yellow < 50 {
		t.Fatalf("blowfish has %d yellow pixels", yellow)
	}
}

func TestInfoBarStartsWithWorkspace(t *testing.T) {
	wm := newTestManager(t)
	ws := wm.GetActiveWorkspace()
	ws.updateInfoBar()

	// the workspace indicator, in the text color, at the bar's left end
	img, err := xproto.GetImage(wm.Conn(), xproto.ImageFormatZPixmap, xproto.Drawable(ws.InfoBarWindow),
		0, 0, 40, uint16(infoBarInnerH()), ^uint32(0)).Reply()
	if err != nil {
		t.Fatal(err)
	}
	bright := 0
	for i := 0; i+3 < len(img.Data); i += 4 {
		if img.Data[i] > 0xc0 && img.Data[i+1] > 0xc0 && img.Data[i+2] > 0xc0 {
			bright++
		}
	}
	if bright < 20 {
		t.Fatalf("%d bright pixels where the workspace indicator should be", bright)
	}
}

func TestBarAlerts(t *testing.T) {
	for _, tc := range []struct {
		cpu, mem, load float64
		cores          int
		want           [3]bool
	}{
		{10, 50, 1, 4, [3]bool{false, false, false}},
		{80, 90, 4, 4, [3]bool{true, true, false}},
		{79.9, 89.9, 4.01, 4, [3]bool{false, false, true}},
		{-1, -1, -1, 0, [3]bool{false, false, false}}, // unknown
	} {
		c, m, l := barAlerts(tc.cpu, tc.mem, tc.load, tc.cores)
		if got := [3]bool{c, m, l}; got != tc.want {
			t.Errorf("barAlerts(%v, %v, %v, %d) = %v, want %v", tc.cpu, tc.mem, tc.load, tc.cores, got, tc.want)
		}
	}
}

func TestBarForms(t *testing.T) {
	st := barState{now: time.Date(2026, 9, 25, 13, 45, 0, 0, time.UTC), cores: 14, cpuPercent: 25.5,
		memPercent: 70, memUsed: 45 << 30, memTotal: 64 << 30, load: [3]float64{3.9, 3.7, 3.5}, hasLoad: true,
		recording: -1, position: "[01:01/01]", system: "OpenBSD/7.9 (arm64)"}
	width := func(form int) int {
		before, after, clock := barPieces(st, form)
		return textWidth(before) + textWidth(after) + len(clock)*barFont.charW
	}
	for form := 1; form < barForms; form++ {
		if width(form) >= width(form-1) {
			t.Fatalf("form %d is %d wide, no shorter than form %d, %d", form, width(form), form-1, width(form-1))
		}
	}
	// the alerts stay in the shortest form
	st.cpuPercent = 95
	_, after, _ := barPieces(st, barForms-1)
	if after[0].s != "CPU: 95%" || !after[0].alert {
		t.Fatalf("shortest form: %+v", after)
	}
}
