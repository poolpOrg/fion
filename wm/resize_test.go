package wm

import (
	"os"
	"strings"
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestResizeTiled(t *testing.T) {
	wm := newTestManager(t)
	mod, shift := wm.KeyboardManager.Mod, uint16(xproto.ModMaskShift)
	ws := wm.GetActiveWorkspace()
	newTestClient(t, wm)
	press(t, wm, XK_Right, mod|shift) // a new frame on the right, active
	left, right := ws.Root.children[0], ws.Root.children[1]
	w0 := right.g.W

	// Mod++, as + with Shift on the US layout: Left grows the right frame
	press(t, wm, XK_Plus, mod|shift)
	if wm.mode == nil || !strings.Contains(wm.GetActiveScreen().prompt.text, "grow") {
		t.Fatalf("Mod++ didn't enter the resize mode")
	}
	press(t, wm, XK_Left, 0)
	if right.g.W != w0+uint16(resizeStep()) || left.g.W+right.g.W != ws.Root.g.W {
		t.Fatalf("right frame %d wide after growing left, want %d", right.g.W, int(w0)+resizeStep())
	}
	checkTree(t, ws)
	// the screen's edge doesn't move
	press(t, wm, XK_Right, 0)
	if right.g.W != w0+uint16(resizeStep()) {
		t.Fatalf("growing towards the screen's edge resized the frame")
	}
	// - switches to shrinking
	press(t, wm, XK_Minus, 0)
	if !strings.Contains(wm.GetActiveScreen().prompt.text, "shrink") {
		t.Fatalf("- didn't switch to shrinking")
	}
	press(t, wm, XK_Left, 0)
	press(t, wm, XK_Left, 0)
	if right.g.W != w0-uint16(resizeStep()) {
		t.Fatalf("right frame %d wide after shrinking twice, want %d", right.g.W, int(w0)-resizeStep())
	}
	// Escape undoes it all
	press(t, wm, XK_Escape, 0)
	if wm.mode != nil || right.g.W != w0 {
		t.Fatalf("Escape left the mode on %v, the frame %d wide, want %d", wm.mode != nil, right.g.W, w0)
	}
	checkTree(t, ws)

	// Return keeps it
	press(t, wm, XK_Minus, mod)
	press(t, wm, XK_Left, 0)
	press(t, wm, XK_Return, 0)
	if wm.mode != nil || right.g.W != w0-uint16(resizeStep()) {
		t.Fatalf("Return didn't keep the resize")
	}

	// in a nested frame, the edge on the left is the root's border
	press(t, wm, XK_Down, mod|shift)
	bottomRight := ws.ActiveFrame
	before := bottomRight.g.W
	press(t, wm, XK_Plus, mod|shift)
	press(t, wm, XK_Left, 0)
	press(t, wm, XK_Return, 0)
	if bottomRight.g.W != before+uint16(resizeStep()) || right.g.W != bottomRight.g.W {
		t.Fatalf("growing a nested frame left didn't move the root's border")
	}
	checkTree(t, ws)

	// never below the smallest size
	press(t, wm, XK_Minus, mod)
	for range 100 {
		press(t, wm, XK_Left, 0)
	}
	press(t, wm, XK_Return, 0)
	if int(right.g.W) < minFrameSize() || int(left.g.W) < minFrameSize() {
		t.Fatalf("frames %d and %d wide, below %d", left.g.W, right.g.W, minFrameSize())
	}
	checkTree(t, ws)
}

func TestScratchpadMoveResize(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	wm := newTestManager(t)
	mod, shift := wm.KeyboardManager.Mod, uint16(xproto.ModMaskShift)
	s := wm.GetActiveScreen()

	// only the scratchpad moves
	press(t, wm, XK_m, mod)
	if wm.mode != nil {
		t.Fatalf("Mod+m entered the move mode on a tiled frame")
	}

	if err := s.toggleScratchpad(); err != nil {
		t.Fatal(err)
	}
	sp := s.scratchpad
	g0 := sp.g
	press(t, wm, XK_m, mod)
	press(t, wm, XK_Right, 0)
	press(t, wm, XK_Down, 0)
	if sp.g.X != g0.X+int16(resizeStep()) || sp.g.Y != g0.Y+int16(resizeStep()) || sp.g.W != g0.W {
		t.Fatalf("scratchpad at %+v after moving right and down from %+v", sp.g, g0)
	}
	// Escape puts it back
	press(t, wm, XK_Escape, 0)
	if sp.g != g0 {
		t.Fatalf("scratchpad at %+v after Escape, want %+v", sp.g, g0)
	}

	// moved, then grown on its left, kept with Return
	press(t, wm, XK_m, mod)
	press(t, wm, XK_Right, 0)
	press(t, wm, XK_Return, 0)
	press(t, wm, XK_Plus, mod|shift)
	press(t, wm, XK_Left, 0)
	press(t, wm, XK_Return, 0)
	want := Geometry{X: g0.X, Y: g0.Y, W: g0.W + uint16(resizeStep()), H: g0.H}
	if sp.g != want {
		t.Fatalf("scratchpad at %+v, want %+v", sp.g, want)
	}
	attr, err := xproto.GetGeometry(wm.Conn(), xproto.Drawable(sp.window)).Reply()
	if err != nil || attr.X != want.X || attr.Width != want.W {
		t.Fatalf("scratchpad window at %+v, want %+v", attr, want)
	}

	// kept for the next time fion starts
	if got := s.scratchpadGeometry(); got != want {
		t.Fatalf("scratchpad would start at %+v, want %+v", got, want)
	}
	// ignored when it doesn't fit the screen anymore
	if err := os.WriteFile(scratchpadPath(), []byte("5000 10 400 300\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := s.scratchpadGeometry(); got.X == 5000 {
		t.Fatalf("scratchpad would start off the screen")
	}

	// kept on the screen when moved against its edge
	press(t, wm, XK_m, mod)
	for range 200 {
		press(t, wm, XK_Up, 0)
	}
	press(t, wm, XK_Return, 0)
	if sp.g.Y != 0 {
		t.Fatalf("scratchpad at y %d after moving it up against the edge", sp.g.Y)
	}
}
