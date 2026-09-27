package wm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"
)

func TestRestartKeepsLayout(t *testing.T) {
	wm := newTestManager(t)
	s := wm.GetActiveScreen()
	a := newTestClient(t, wm)
	b := newTestClient(t, wm)
	if err := wm.GetActiveFrame().split(true, false); err != nil {
		t.Fatal(err)
	}
	ws1 := s.GetActiveWorkspace()
	ws1.Root.ratio = 0.3
	ws1.Root.layout()
	c := newTestClient(t, wm) // in the right half
	right := ws1.ActiveFrame
	// split too, so that the active frame, the left one, isn't the last
	// one made
	if err := right.split(false, false); err != nil {
		t.Fatal(err)
	}
	right = right.children[0]
	ws1.Root.children[0].selectClient(0) // a
	if err := wm.createWorkspace(); err != nil {
		t.Fatal(err)
	}
	ws2 := s.GetActiveWorkspace()
	ws2.name = "fion"
	d := newTestClient(t, wm)
	if err := s.toggleScratchpad(); err != nil {
		t.Fatal(err)
	}
	e := newTestClient(t, wm)
	s.toggleScratchpad()
	s.showWorkspace(ws1)
	ws1.ActiveFrame = ws1.Root.children[0]
	drainEvents(t, wm)

	// written down, fion gone, a new one reading it
	path := filepath.Join(t.TempDir(), "layout.json")
	b1, _ := json.Marshal(wm.layout())
	os.WriteFile(path, b1, 0o600)
	wm.Close()
	t.Setenv("FION_RESTORE", path)
	wm2 := newTestManager(t)
	drainEvents(t, wm2)
	s2 := wm2.GetActiveScreen()

	if len(s2.Workspaces) != 2 || s2.Workspaces[1].name != "fion" || s2.GetActiveWorkspace() != s2.Workspaces[0] {
		t.Fatalf("workspaces not restored: %d, %q", len(s2.Workspaces), s2.Workspaces[len(s2.Workspaces)-1].name)
	}
	r := s2.Workspaces[0].Root
	if r.leaf || !r.vertical || r.ratio != 0.3 {
		t.Fatalf("the split wasn't restored: leaf %v vertical %v ratio %v", r.leaf, r.vertical, r.ratio)
	}
	frameOf := func(w xproto.Window) *Frame {
		c, ok := wm2.Clients[w]
		if !ok {
			t.Fatalf("0x%x not managed after the restart", w)
		}
		return c.frame
	}
	if frameOf(a) != r.children[0] || frameOf(b) != r.children[0] || frameOf(c) != r.children[1].children[0] {
		t.Fatalf("windows in the wrong frames")
	}
	if l := r.children[0]; l.clients[0] != a || l.clients[1] != b || l.activeClient != 0 {
		t.Fatalf("tabs out of order, or the wrong one active: %v, %d", l.clients, l.activeClient)
	}
	if s2.Workspaces[0].ActiveFrame != r.children[0] {
		t.Fatalf("the active frame wasn't restored")
	}
	if frameOf(d).workspace != s2.Workspaces[1] {
		t.Fatalf("the second workspace's window moved")
	}
	if !frameOf(e).floating() || s2.scratchpadShown {
		t.Fatalf("the scratchpad's window moved, or it shows")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the layout file was kept")
	}
}

func TestSessionMenu(t *testing.T) {
	wm := newTestManager(t)
	mod := wm.KeyboardManager.Mod
	press(t, wm, XK_Escape, mod)
	if !promptShown(t, wm) || wm.exiting != exitNone {
		t.Fatalf("Mod+Escape didn't ask")
	}
	press(t, wm, XK_x, 0)
	if promptShown(t, wm) || wm.exiting != exitNone {
		t.Fatalf("another key didn't cancel")
	}
	press(t, wm, XK_Escape, mod)
	press(t, wm, XK_q, 0)
	if wm.exiting != exitQuit {
		t.Fatalf("q didn't quit")
	}

	// restart writes the layout, for the next fion
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	wm.exiting = exitNone
	wm.restart()
	if wm.exiting != exitRestart {
		t.Fatalf("restart didn't ask Run to return")
	}
	if _, err := loadLayout(wm.restartPath); err != nil {
		t.Fatalf("layout written: %v", err)
	}

	sg := wm.GetActiveScreen().Geometry()
	if !onPowerButton(int(sg.W)-5, int(sg.W)) || onPowerButton(int(sg.W)/2, int(sg.W)) {
		t.Fatalf("the power button's place")
	}
}

func TestLogout(t *testing.T) {
	wm := newTestManager(t)
	// windows that can't be asked are killed, their clients gone
	newTestClient(t, wm)
	newTestClient(t, wm)
	drainEvents(t, wm)
	wm.logout()
	for range 50 {
		drainEvents(t, wm)
		if len(wm.Clients) == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	wm.checkLogout()
	if wm.exiting != exitQuit {
		t.Fatalf("logout didn't quit once the windows were gone: %d left", len(wm.Clients))
	}

	// a window that stays cancels it
	wm.Close()
	wm2 := newTestManager(t)
	newTestClient(t, wm2)
	drainEvents(t, wm2)
	wm2.logoutUntil = time.Now().Add(-time.Second)
	wm2.checkLogout()
	if wm2.exiting != exitNone || !wm2.logoutUntil.IsZero() {
		t.Fatalf("logout wasn't canceled")
	}
	if last := wm2.notes.history[len(wm2.notes.history)-1]; !strings.Contains(last.text, "logout canceled") {
		t.Fatalf("message %q", last.text)
	}
}
