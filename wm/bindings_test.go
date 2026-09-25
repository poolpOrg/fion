package wm

import (
	"strings"
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestBindingsFrames(t *testing.T) {
	wm := newTestManager(t)
	mod, shift := wm.KeyboardManager.Mod, uint16(xproto.ModMaskShift)
	ws := wm.GetActiveWorkspace()
	a := newTestClient(t, wm)

	// Mod+Shift+Right: a new, active frame on the right, the client left
	press(t, wm, XK_Right, mod|shift)
	checkTree(t, ws)
	left, right := ws.Root.children[0], ws.Root.children[1]
	if ws.ActiveFrame != right || len(right.clients) != 0 || wm.Clients[a].frame != left {
		t.Fatalf("after Mod+Shift+Right, the new frame isn't the empty active one on the right")
	}

	// Mod+Left goes back, Mod+Left again has nowhere to go
	press(t, wm, XK_Left, mod)
	if ws.ActiveFrame != left {
		t.Fatalf("Mod+Left didn't go to the left frame")
	}
	press(t, wm, XK_Left, mod)
	if ws.ActiveFrame != left {
		t.Fatalf("Mod+Left moved past the screen's edge")
	}

	// Mod+Shift+Up on the left: a new frame above, the client below
	press(t, wm, XK_Up, mod|shift)
	checkTree(t, ws)
	top, bottom := left.children[0], left.children[1]
	if ws.ActiveFrame != top || wm.Clients[a].frame != bottom {
		t.Fatalf("after Mod+Shift+Up, the new frame isn't the active one above")
	}
	press(t, wm, XK_Down, mod)
	if ws.ActiveFrame != bottom {
		t.Fatalf("Mod+Down didn't go to the frame below")
	}
	// from the lower left, Right is the right frame, which it touches
	press(t, wm, XK_Right, mod)
	if ws.ActiveFrame != right {
		t.Fatalf("Mod+Right didn't go to the right frame")
	}
}

func TestBindingsTabsAndWorkspaces(t *testing.T) {
	wm := newTestManager(t)
	mod, shift := wm.KeyboardManager.Mod, uint16(xproto.ModMaskShift)
	f := wm.GetActiveFrame()
	for range 3 {
		newTestClient(t, wm)
	}
	drainEvents(t, wm)

	// the last one opened is active; Tab wraps to the first, Shift+Tab back
	press(t, wm, XK_Tab, mod)
	if f.activeClient != 0 {
		t.Fatalf("active tab %d after Mod+Tab, want 0", f.activeClient)
	}
	press(t, wm, XK_Tab, mod|shift)
	if f.activeClient != 2 {
		t.Fatalf("active tab %d after Mod+Shift+Tab, want 2", f.activeClient)
	}
	checkTabs(t, f)

	s := wm.GetActiveScreen()
	press(t, wm, XK_w, mod)
	if len(s.Workspaces) != 2 || s.GetActiveWorkspace() != s.Workspaces[1] {
		t.Fatalf("Mod+w didn't make and show a second workspace")
	}
	press(t, wm, XK_Prior, mod)
	if s.GetActiveWorkspace() != s.Workspaces[0] {
		t.Fatalf("Mod+Page Up didn't go back to the first workspace")
	}
	press(t, wm, XK_Next, mod)
	if s.GetActiveWorkspace() != s.Workspaces[1] {
		t.Fatalf("Mod+Page Down didn't go to the second workspace")
	}
}

func TestCheatSheet(t *testing.T) {
	wm := newTestManager(t)
	mod := wm.KeyboardManager.Mod
	lines := strings.Join(cheatSheetLines("Super"), "\n")
	for _, b := range bindings {
		if !strings.Contains(lines, b.keys("Super")) || !strings.Contains(lines, b.desc) {
			t.Errorf("cheat sheet lacks %s: %s", b.keys("Super"), b.desc)
		}
	}
	for _, want := range []string{"Super+Shift+Tab", "Super+Page Down", "Super+?"} {
		if !strings.Contains(lines, want) {
			t.Errorf("cheat sheet lacks %s", want)
		}
	}

	shown := func() bool {
		attr, err := xproto.GetWindowAttributes(wm.Conn(), wm.cheat.window).Reply()
		if err != nil {
			t.Fatal(err)
		}
		return attr.MapState == xproto.MapStateViewable
	}
	// ?, with Shift as the US layout wants it
	press(t, wm, XK_question, mod|xproto.ModMaskShift)
	if !wm.cheatSheetShown() || !shown() {
		t.Fatalf("Mod+? didn't show the cheat sheet")
	}
	press(t, wm, XK_x, 0)
	if wm.cheatSheetShown() || shown() {
		t.Fatalf("a key didn't close the cheat sheet")
	}
}
