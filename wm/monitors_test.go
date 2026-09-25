package wm

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestParseMonitors(t *testing.T) {
	ms, err := parseMonitors("1920x1080+0+0, 1280x1024+1920+0")
	if err != nil || len(ms) != 2 || ms[1].g != (Geometry{1920, 0, 1280, 1024}) || !ms[0].primary {
		t.Fatalf("parsed %+v, %v", ms, err)
	}
	for _, bad := range []string{"", "1920x1080", "0x0+0+0", "1920x1080+0+0,foo"} {
		if _, err := parseMonitors(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestCleanMonitors(t *testing.T) {
	ms := cleanMonitors([]monitor{
		{name: "screen", g: Geometry{0, 0, 3840, 1080}},
		{name: "C", g: Geometry{2560, 0, 1280, 800}},
		{name: "B", g: Geometry{1280, 0, 1280, 1080}},
		{name: "A", g: Geometry{0, 0, 1280, 1080}},
		{name: "A-mirror", g: Geometry{0, 0, 1280, 1080}},
	})
	var names []string
	for _, m := range ms {
		names = append(names, m.name)
	}
	if len(names) != 3 || names[0] != "A" || names[1] != "B" || names[2] != "C" {
		t.Fatalf("kept %v, want the monitors from left to right, without the whole screen and mirrors", names)
	}
}

// newThreeMonitors has fion manage the test display as three monitors side
// by side, the third lower than the others.
func newThreeMonitors(t *testing.T) *Manager {
	t.Helper()
	t.Setenv("FION_MONITORS", "400x600+0+0,400x600+400+0,480x500+800+0")
	return newTestManager(t)
}

func TestMonitors(t *testing.T) {
	wm := newThreeMonitors(t)
	if len(wm.Screens) != 3 {
		t.Fatalf("%d screens, want 3", len(wm.Screens))
	}
	for i, s := range wm.Screens {
		g := s.Geometry()
		ws := s.GetActiveWorkspace()
		wg, err := xproto.GetGeometry(wm.Conn(), xproto.Drawable(ws.WorkspaceWindow)).Reply()
		if err != nil {
			t.Fatal(err)
		}
		if wg.X != g.X || wg.Y != g.Y || wg.Width != g.W || wg.Height != g.H {
			t.Fatalf("workspace of monitor %d at %+v, want %+v", i, wg, g)
		}
		if int(ws.Root.g.W) != int(g.W) || int(ws.Root.g.H) != int(g.H)-infoBarOuterH() {
			t.Fatalf("frames of monitor %d are %+v", i, ws.Root.g)
		}
		if x, y := ws.Root.origin(); x != g.X || y != g.Y {
			t.Fatalf("frame of monitor %d at %d,%d in the root", i, x, y)
		}
		if f, _, _ := wm.frameAt(g.X+10, g.Y+10); f != ws.Root {
			t.Fatalf("frameAt on monitor %d found %v", i, f)
		}
		if _, _, n := ws.position(); n != 1 {
			t.Fatalf("monitor %d has %d workspaces", i, n)
		}
	}
	if wm.GetActiveScreen() != wm.Screens[0] {
		t.Fatalf("the first monitor isn't the active one")
	}
}

func TestFocusAcrossMonitors(t *testing.T) {
	wm := newThreeMonitors(t)
	mod := wm.KeyboardManager.Mod
	left, middle := wm.Screens[0], wm.Screens[1]
	drainEvents(t, wm)

	// Right, past the first monitor's edge
	press(t, wm, XK_Right, mod)
	if wm.GetActiveScreen() != middle {
		t.Fatalf("Mod+Right didn't reach the second monitor")
	}
	if left.GetActiveWorkspace().Root.isActive() || !middle.GetActiveWorkspace().Root.isActive() {
		t.Fatalf("the active frame shows on the wrong monitor")
	}
	// a new window opens there
	win := newTestClient(t, wm)
	if wm.Clients[win].frame.screen != middle {
		t.Fatalf("new window on monitor %v, not the active one", wm.Clients[win].frame.screen.monitor.name)
	}
	// a split frame still reaches across: its right half to the third
	press(t, wm, XK_Right, mod|xproto.ModMaskShift)
	press(t, wm, XK_Right, mod)
	if wm.GetActiveScreen() != wm.Screens[2] {
		t.Fatalf("Mod+Right from the right half didn't reach the third monitor")
	}
	press(t, wm, XK_Left, mod)
	if wm.GetActiveScreen() != middle || wm.GetActiveFrame() != middle.GetActiveWorkspace().Root.children[1] {
		t.Fatalf("Mod+Left came back to %v", wm.GetActiveFrame())
	}
	// no monitor further left
	press(t, wm, XK_Left, mod)
	press(t, wm, XK_Left, mod)
	press(t, wm, XK_Left, mod)
	if wm.GetActiveScreen() != left {
		t.Fatalf("Mod+Left didn't stop on the first monitor")
	}
}

func TestPanelOnItsMonitor(t *testing.T) {
	wm := newThreeMonitors(t)
	s := wm.Screens[2]
	wm.setActiveScreen(s)
	if err := s.togglePanel(); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	pg, err := xproto.GetGeometry(wm.Conn(), xproto.Drawable(s.panel.window)).Reply()
	if err != nil {
		t.Fatal(err)
	}
	g := s.Geometry()
	if pg.X != g.X || int(pg.Y)+int(pg.Height) != int(g.Y)+int(g.H)-infoBarOuterH() || pg.Width != g.W {
		t.Fatalf("panel at %+v on the monitor %+v", pg, g)
	}
	if wm.Screens[0].GetActiveWorkspace().Root.g.H != uint16(wm.Screens[0].workAreaH()) ||
		wm.Screens[0].workAreaH() != int(wm.Screens[0].Geometry().H)-infoBarOuterH() {
		t.Fatalf("the panel shrank the frames of another monitor")
	}
	s.togglePanel()
}

func TestMonitorsChange(t *testing.T) {
	wm := newThreeMonitors(t)
	third := wm.Screens[2]
	wm.setActiveScreen(third)
	win := newTestClient(t, wm)
	if err := third.toggleScratchpad(); err != nil {
		t.Fatal(err)
	}
	pad := newTestClient(t, wm)
	drainEvents(t, wm)

	// the third monitor unplugged, the second one wider
	t.Setenv("FION_MONITORS", "400x600+0+0,600x600+400+0")
	wm.updateMonitors()
	drainEvents(t, wm)
	if len(wm.Screens) != 2 {
		t.Fatalf("%d screens after unplugging one", len(wm.Screens))
	}
	first := wm.Screens[0]
	if len(first.Workspaces) != 2 || wm.Clients[win].frame.screen != first {
		t.Fatalf("the unplugged monitor's workspace and window didn't move to the first")
	}
	if c := wm.Clients[pad]; c == nil || c.frame.floating() || c.frame.screen != first {
		t.Fatalf("the unplugged monitor's scratchpad tab wasn't kept")
	}
	if g := wm.Screens[1].GetActiveWorkspace().Root.g; g.W != 600 {
		t.Fatalf("the second monitor's frames are %d wide, want 600", g.W)
	}
	checkTree(t, first.Workspaces[1])

	// and plugged back
	t.Setenv("FION_MONITORS", "400x600+0+0,400x600+400+0,480x500+800+0")
	wm.updateMonitors()
	if len(wm.Screens) != 3 || len(wm.Screens[2].Workspaces) != 1 {
		t.Fatalf("plugging a monitor back: %d screens", len(wm.Screens))
	}
}
