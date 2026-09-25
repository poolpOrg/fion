package wm

import (
	"log"
	"strings"

	"github.com/jezek/xgb/xproto"
)

// fion draws with the Dracula palette, https://draculatheme.com, and makes
// it the default for the terminals it starts. Colors are pixel values for a
// 24-bit TrueColor visual.
const (
	draculaBackground  = 0x282a36
	draculaDarker      = 0x21222c
	draculaCurrentLine = 0x44475a
	draculaForeground  = 0xf8f8f2
	draculaComment     = 0x6272a4
	draculaPurple      = 0xbd93f9
)

const (
	colorBackground = draculaBackground  // root, workspaces
	colorBar        = draculaDarker      // info bar, title bars, launcher
	colorBorder     = draculaCurrentLine // borders of inactive frames
	colorText       = draculaForeground
	colorDim        = draculaComment // secondary text

	colorTab         = draculaCurrentLine // tabs
	colorTabSelected = draculaComment     // active tab of an inactive frame
	colorAccent      = draculaPurple      // active tab, active frame, selection
	colorAccentText  = draculaBackground  // text on colorAccent

	colorAlert = 0xff5555 // Dracula's red, for values past their threshold

	// empty frames are black, with the logo in its own white
	colorEmpty = 0x000000
	colorLogo  = 0xffffff
)

// the info bar shows in bold red a CPU or memory use from these, a load
// average above the number of CPUs
const (
	alertCPUPercent = 80
	alertMemPercent = 90

	// and a battery at or below this, on battery
	alertBatteryPercent = 15
)

// xtermResources are the Dracula colors for xterm, from Dracula's
// Xresources port, and UTF-8 whatever the locale: XQuartz's Xlib, for one,
// supports none, and xterm then falls back to Latin-1, which scrambles the
// output of programs drawing with UTF-8, such as btop, in a font that has
// its characters.
var xtermResources = []string{
	"XTerm*locale: false",
	"XTerm*utf8: 2",
	"XTerm*background: #282A36",
	"XTerm*foreground: #F8F8F2",
	"XTerm*cursorColor: #F8F8F2",
	"XTerm*color0: #000000",
	"XTerm*color8: #4D4D4D",
	"XTerm*color1: #FF5555",
	"XTerm*color9: #FF6E67",
	"XTerm*color2: #50FA7B",
	"XTerm*color10: #5AF78E",
	"XTerm*color3: #F1FA8C",
	"XTerm*color11: #F4F99D",
	"XTerm*color4: #BD93F9",
	"XTerm*color12: #CAA9FA",
	"XTerm*color5: #FF79C6",
	"XTerm*color13: #FF92D0",
	"XTerm*color6: #8BE9FD",
	"XTerm*color14: #9AEDFE",
	"XTerm*color7: #BFBFBF",
	"XTerm*color15: #E6E6E6",
}

// xtermTheme is xtermResources and the Unicode variant of fion's font:
// xterm then matches fion's size, and some builds, such as Homebrew's,
// otherwise pick a font without braille or block elements.
func xtermTheme() []string {
	return append([]string{"XTerm*font: " + font.unicode}, xtermResources...)
}

// resourceAttribute returns the last component of a resource line's name:
// "background" for "XTerm*vt100.background: black".
func resourceAttribute(line string) string {
	name, _, ok := strings.Cut(line, ":")
	if !ok {
		return ""
	}
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, ".*"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// mergeResources adds to a resource database the lines setting attributes
// it doesn't set yet, for any application, so that the user's own
// resources win.
func mergeResources(database string, lines []string) string {
	set := map[string]bool{}
	for _, line := range strings.Split(database, "\n") {
		if a := resourceAttribute(line); a != "" && !strings.HasPrefix(strings.TrimSpace(line), "!") {
			set[a] = true
		}
	}
	merged := strings.TrimRight(database, "\n")
	for _, line := range lines {
		if set[resourceAttribute(line)] {
			continue
		}
		if merged != "" {
			merged += "\n"
		}
		merged += line
	}
	if merged != "" {
		merged += "\n"
	}
	return merged
}

// installXtermTheme adds the Dracula colors for xterm to the display's
// resource database, which the terminals started from now on read, fion's
// or not.
func (wm *Manager) installXtermTheme() {
	root := wm.Setup.Roots[0].Root
	r, err := xproto.GetProperty(wm.Conn(), false, root, xproto.AtomResourceManager,
		xproto.AtomString, 0, ^uint32(0)).Reply()
	if err != nil {
		log.Printf("resources: %v", err)
		return
	}
	database := string(r.Value[:r.ValueLen])
	merged := mergeResources(database, xtermTheme())
	if merged == database {
		return
	}
	xproto.ChangeProperty(wm.Conn(), xproto.PropModeReplace, root, xproto.AtomResourceManager,
		xproto.AtomString, 8, uint32(len(merged)), []byte(merged))
}
