package wm

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

func checkFocus(t *testing.T, wm *Manager, want xproto.Window) {
	t.Helper()
	r, err := xproto.GetInputFocus(wm.Conn()).Reply()
	if err != nil {
		t.Fatal(err)
	}
	if r.Focus != want {
		t.Fatalf("input focus on 0x%x, want 0x%x", r.Focus, want)
	}
}

func checkScratchpadShown(t *testing.T, s *Screen, shown bool) {
	t.Helper()
	sp := s.scratchpad
	attr, err := xproto.GetWindowAttributes(s.Conn(), sp.window).Reply()
	if err != nil {
		t.Fatal(err)
	}
	if viewable := attr.MapState == xproto.MapStateViewable; viewable != shown {
		t.Fatalf("scratchpad viewable=%v, want %v", viewable, shown)
	}
	if !shown {
		return
	}

	tree, err := xproto.QueryTree(s.Conn(), s.Info().Root).Reply()
	if err != nil {
		t.Fatal(err)
	}
	if tree.Children[len(tree.Children)-1] != sp.window {
		t.Fatalf("scratchpad is not on top of the root's children")
	}
}

func TestScratchpad(t *testing.T) {
	wm := newTestManager(t)
	s := wm.GetActiveScreen()
	tiled := wm.GetActiveFrame()

	if err := s.toggleScratchpad(); err != nil {
		t.Fatal(err)
	}
	sp := s.scratchpad
	checkScratchpadShown(t, s, true)
	if wm.GetActiveFrame() != sp || !sp.isActive() || tiled.isActive() {
		t.Fatalf("the shown scratchpad is not the active frame")
	}

	// centered on the screen, border included, and on the root
	geom, err := xproto.GetGeometry(wm.Conn(), xproto.Drawable(sp.window)).Reply()
	if err != nil {
		t.Fatal(err)
	}
	sg := s.Geometry()
	outerW, outerH := int(geom.Width)+2, int(geom.Height)+2
	if int(geom.X)*2+outerW != int(sg.W) || int(geom.Y)*2+outerH != int(sg.H) {
		t.Fatalf("scratchpad at %+v, not centered on %dx%d", geom, sg.W, sg.H)
	}
	if tree, _ := xproto.QueryTree(wm.Conn(), sp.window).Reply(); tree.Parent != s.Info().Root {
		t.Fatalf("scratchpad is not a child of the root")
	}
	checkFocus(t, wm, wm.noFocus)

	// new windows open in the shown scratchpad, which gives them the focus
	a := newTestClient(t, wm)
	b := newTestClient(t, wm)
	drainEvents(t, wm)
	if wm.Clients[a].frame != sp || wm.Clients[b].frame != sp {
		t.Fatalf("new windows did not open in the scratchpad")
	}
	checkTabs(t, sp)
	checkFocus(t, wm, b)
	sp.cycleClientRight()
	drainEvents(t, wm)
	checkFocus(t, wm, a)

	// it can be neither split nor removed
	if err := sp.splitH(); err == nil {
		t.Fatalf("split the scratchpad")
	}
	checkTree(t, wm.GetActiveWorkspace())

	// a workspace created while it is shown stays below it
	ws, err := s.newWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	ws.Map()
	drainEvents(t, wm)
	checkScratchpadShown(t, s, true)

	// hiding it gives the focus back to the pointer and the frames back
	// to the workspace, and keeps its clients
	if err := s.toggleScratchpad(); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	checkScratchpadShown(t, s, false)
	if wm.GetActiveFrame() != ws.ActiveFrame || sp.isActive() || !ws.ActiveFrame.isActive() {
		t.Fatalf("the workspace's frame is not active once the scratchpad is hidden")
	}
	checkFocus(t, wm, wm.noFocus)
	if len(wm.Clients) != 2 {
		t.Fatalf("%d clients managed after hiding the scratchpad, want 2", len(wm.Clients))
	}

	// showing it again brings its active client back into focus
	if err := s.toggleScratchpad(); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	checkScratchpadShown(t, s, true)
	checkTabs(t, sp)
	checkFocus(t, wm, a)

	// closing its clients empties it, and then it stays
	for range 2 {
		if err := wm.closeActive(); err != nil {
			t.Fatal(err)
		}
		drainEvents(t, wm)
	}
	if len(sp.clients) != 0 {
		t.Fatalf("scratchpad still holds %d clients", len(sp.clients))
	}
	checkFocus(t, wm, wm.noFocus)
	if err := wm.closeActive(); err == nil {
		t.Fatalf("removed the scratchpad")
	}
	checkScratchpadShown(t, s, true)
}
