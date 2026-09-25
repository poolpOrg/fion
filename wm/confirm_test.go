package wm

import (
	"strings"
	"testing"

	"github.com/jezek/xgb/xproto"
)

// press has fion handle a key press, as the event loop would.
func press(t *testing.T, wm *Manager, sym xproto.Keysym, state uint16) {
	t.Helper()
	kcs := wm.KeyboardManager.keycodesForSym(sym)
	if len(kcs) == 0 {
		t.Fatalf("no keycode for keysym 0x%x", sym)
	}
	wm.handleKeyPress(xproto.KeyPressEvent{Detail: kcs[0], State: state, Root: wm.GetActiveScreen().Info().Root})
	drainEvents(t, wm)
}

func promptShown(t *testing.T, wm *Manager) bool {
	t.Helper()
	p := wm.GetActiveScreen().prompt
	if p == nil {
		return false
	}
	attr, err := xproto.GetWindowAttributes(wm.Conn(), p.window).Reply()
	if err != nil {
		t.Fatal(err)
	}
	return attr.MapState == xproto.MapStateViewable
}

func TestCloseConfirmWindow(t *testing.T) {
	wm := newTestManager(t)
	mod := wm.KeyboardManager.Mod
	_, win := newClosableClient(t, wm, true)

	// Mod+d asks; any other key cancels
	press(t, wm, XK_d, mod)
	if wm.confirm == nil || !promptShown(t, wm) || !strings.HasPrefix(wm.confirm.text, "Close") {
		t.Fatalf("no question after Mod+d")
	}
	press(t, wm, XK_x, 0)
	if wm.confirm != nil || promptShown(t, wm) {
		t.Fatalf("question still up after another key")
	}
	if c := wm.Clients[win]; c == nil || c.closeRequested {
		t.Fatalf("the window was closed on a cancel")
	}

	// a modifier alone doesn't answer; d confirms
	press(t, wm, XK_d, mod)
	press(t, wm, 0xFFE1, 0) // Shift_L
	if wm.confirm == nil {
		t.Fatalf("Shift answered the question")
	}
	press(t, wm, XK_d, 0)
	if wm.confirm != nil || promptShown(t, wm) {
		t.Fatalf("question still up after d")
	}
	if c := wm.Clients[win]; c == nil || !c.closeRequested {
		t.Fatalf("the window wasn't asked to close")
	}

	// again: offered to kill it, and Mod+d confirms as d does
	press(t, wm, XK_d, mod)
	if wm.confirm == nil || !strings.HasPrefix(wm.confirm.text, "Kill") {
		t.Fatalf("question %+v, want to kill", wm.confirm)
	}
	press(t, wm, XK_d, mod)
	waitForgotten(t, wm, win)
}

func TestCloseConfirmFrameAndWorkspace(t *testing.T) {
	wm := newTestManager(t)
	mod := wm.KeyboardManager.Mod
	s := wm.GetActiveScreen()

	// the last workspace, a single empty frame: nothing to ask
	press(t, wm, XK_d, mod)
	if wm.confirm != nil || promptShown(t, wm) {
		t.Fatalf("question with nothing to close")
	}

	// an empty frame
	ws := wm.GetActiveWorkspace()
	if err := ws.splitV(); err != nil {
		t.Fatal(err)
	}
	press(t, wm, XK_d, mod)
	if wm.confirm == nil || wm.confirm.text[:len("Remove this empty frame?")] != "Remove this empty frame?" {
		t.Fatalf("question %+v, want to remove the frame", wm.confirm)
	}
	press(t, wm, XK_d, 0)
	if !ws.Root.leaf {
		t.Fatalf("frame not removed")
	}
	checkTree(t, ws)

	// a second workspace, a single empty frame
	second, err := s.newWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	second.Map()
	press(t, wm, XK_d, mod)
	if wm.confirm == nil || !strings.HasPrefix(wm.confirm.text, "Remove workspace 2?") {
		t.Fatalf("question %+v, want to remove workspace 2", wm.confirm)
	}
	press(t, wm, XK_d, 0)
	if len(s.Workspaces) != 1 {
		t.Fatalf("%d workspaces after removing one of two", len(s.Workspaces))
	}
}
