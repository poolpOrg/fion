package wm

import (
	"strings"
	"testing"

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
		if ic.h > infoBarH || ic.w > 2*infoBarH {
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

func TestInfoBarLogo(t *testing.T) {
	wm := newTestManager(t)
	ws := wm.GetActiveWorkspace()
	ws.updateInfoBar()

	// the logo is drawn in the text color at the bar's left end
	img, err := xproto.GetImage(wm.Conn(), xproto.ImageFormatZPixmap, xproto.Drawable(ws.InfoBarWindow),
		0, 0, 40, infoBarH, ^uint32(0)).Reply()
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
		t.Fatalf("%d bright pixels where the bar's logo should be", bright)
	}
}
