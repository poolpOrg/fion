package wm

import (
	"os"
	"testing"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// These tests need an X server fion can manage, such as a throwaway Xvfb:
//
//	Xvfb :99 -noreset & FION_TEST_DISPLAY=:99 go test ./wm
//
// They are skipped otherwise, so that they never take over a real display.
// Without -noreset the server resets when a test disconnects, and the next
// test's connection can be dropped.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	display := os.Getenv("FION_TEST_DISPLAY")
	if display == "" {
		t.Skip("FION_TEST_DISPLAY not set")
	}
	t.Setenv("DISPLAY", display)
	wm, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(wm.Close)
	return wm
}

// newTestClient creates a top-level window and has fion manage it, as a
// MapRequest would. The window belongs to its own connection, like a real
// client's: X refuses a client's own windows in its save-set.
func newTestClient(t *testing.T, wm *Manager) xproto.Window {
	t.Helper()
	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)

	scr := wm.GetActiveWorkspace().Screen.Info()
	win, err := xproto.NewWindowId(conn)
	if err != nil {
		t.Fatal(err)
	}
	err = xproto.CreateWindowChecked(conn, scr.RootDepth, win, scr.Root, 0, 0, 100, 100, 0,
		xproto.WindowClassInputOutput, scr.RootVisual, 0, nil).Check()
	if err != nil {
		t.Fatal(err)
	}
	wm.manageWindow(win)
	return win
}

// checkTree verifies the frame tree's invariants and that the X window
// hierarchy and geometries match it.
func checkTree(t *testing.T, ws *Workspace) {
	t.Helper()

	if ws.Root.parent != nil {
		t.Fatalf("root frame has a parent")
	}

	activeFound := false
	walk(ws.Root, func(f *Frame) {
		if f == ws.ActiveFrame {
			activeFound = true
		}
		if f.leaf {
			if len(f.children) != 0 {
				t.Fatalf("leaf 0x%x has %d children", f.window, len(f.children))
			}
		} else {
			if len(f.children) != 2 {
				t.Fatalf("split frame 0x%x has %d children", f.window, len(f.children))
			}
			if len(f.clients) != 0 {
				t.Fatalf("split frame 0x%x holds %d clients", f.window, len(f.clients))
			}
			g1, g2 := f.childGeometries()
			for i, want := range []Geometry{g1, g2} {
				child := f.children[i]
				if child.parent != f {
					t.Fatalf("child 0x%x of 0x%x points to another parent", child.window, f.window)
				}
				if child.g != want {
					t.Fatalf("child 0x%x of 0x%x has geometry %+v, want %+v", child.window, f.window, child.g, want)
				}
			}
		}

		tree, err := xproto.QueryTree(f.Conn(), f.window).Reply()
		if err != nil {
			t.Fatalf("QueryTree 0x%x: %v", f.window, err)
		}
		if tree.Parent != f.parentWindow() {
			t.Fatalf("frame 0x%x is a child of 0x%x, want 0x%x", f.window, tree.Parent, f.parentWindow())
		}
		geom, err := xproto.GetGeometry(f.Conn(), xproto.Drawable(f.window)).Reply()
		if err != nil {
			t.Fatalf("GetGeometry 0x%x: %v", f.window, err)
		}
		if got := (Geometry{X: geom.X, Y: geom.Y, W: geom.Width, H: geom.Height}); got != f.g {
			t.Fatalf("frame 0x%x is at %+v, want %+v", f.window, got, f.g)
		}
	})

	if !activeFound {
		t.Fatalf("active frame is not in the tree")
	}
	if !ws.ActiveFrame.leaf {
		t.Fatalf("active frame is not a leaf")
	}
}

// removeActive removes the active frame the way the Super+w d binding does.
func removeActive(t *testing.T, ws *Workspace) {
	t.Helper()
	f := ws.ActiveFrame
	if err := f.parent.RemoveChild(f); err != nil {
		t.Fatalf("RemoveChild: %v", err)
	}
}

func TestSplitAndRemove(t *testing.T) {
	wm := newTestManager(t)
	ws := wm.GetActiveWorkspace()
	root := ws.Root
	checkTree(t, ws)

	// root -> [top, bottom], bottom -> [left, right], right -> [a, b]
	for _, split := range []func() error{ws.splitH, ws.splitV, ws.splitH} {
		if err := split(); err != nil {
			t.Fatalf("split: %v", err)
		}
		checkTree(t, ws)
	}

	// Remove frames nested below the root first: this used to reparent the
	// sibling to the workspace at (0,0) and leave it out of the tree.
	for ws.Root != ws.ActiveFrame {
		removeActive(t, ws)
		checkTree(t, ws)
	}
	if ws.Root.g != root.g {
		t.Fatalf("last frame has geometry %+v, want the workspace's %+v", ws.Root.g, root.g)
	}
}

func TestRemoveLeavesActiveLeaf(t *testing.T) {
	wm := newTestManager(t)
	ws := wm.GetActiveWorkspace()

	// root -> [top, bottom]; then split top so that bottom's sibling is a
	// split frame, and remove bottom.
	if err := ws.splitH(); err != nil {
		t.Fatal(err)
	}
	bottom := ws.ActiveFrame
	ws.ActiveFrame = ws.Root.children[0]
	if err := ws.splitV(); err != nil {
		t.Fatal(err)
	}
	ws.ActiveFrame = bottom
	removeActive(t, ws)
	checkTree(t, ws)

	// Splitting the active frame must still work.
	if err := ws.splitH(); err != nil {
		t.Fatalf("split after remove: %v", err)
	}
	checkTree(t, ws)
}

func TestSplitKeepsClients(t *testing.T) {
	wm := newTestManager(t)
	ws := wm.GetActiveWorkspace()

	win := newTestClient(t, wm)
	drainEvents(t, wm)

	frame := ws.ActiveFrame
	if err := ws.splitV(); err != nil {
		t.Fatal(err)
	}
	checkTree(t, ws)

	left := ws.Root.children[0]
	if wm.Clients[win].frame != left {
		t.Fatalf("client is not tracked in the frame it moved to")
	}
	if left.GetActiveClient() != win {
		t.Fatalf("frame the client moved to has no active client")
	}
	tree, err := xproto.QueryTree(wm.Conn(), win).Reply()
	if err != nil {
		t.Fatal(err)
	}
	if tree.Parent != left.window {
		t.Fatalf("client is a child of 0x%x, want 0x%x", tree.Parent, left.window)
	}
	if frame.leaf || len(frame.clients) != 0 {
		t.Fatalf("split frame still holds clients")
	}

	// A frame holding a client can't be removed.
	if err := ws.Root.RemoveChild(left); err == nil {
		t.Fatalf("removed a frame holding a client")
	}
	checkTree(t, ws)
}

// destroyTestClient destroys a window made by newTestClient, from another
// connection as a client exiting would.
func destroyTestClient(t *testing.T, win xproto.Window) {
	t.Helper()
	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := xproto.DestroyWindowChecked(conn, win).Check(); err != nil {
		t.Fatal(err)
	}
}

// drainEvents feeds the events the server has queued so far to fion.
func drainEvents(t *testing.T, wm *Manager) {
	t.Helper()
	// a round trip, so that the events caused by earlier requests are in
	if _, err := xproto.GetInputFocus(wm.Conn()).Reply(); err != nil {
		t.Fatal(err)
	}
	for {
		ev, err := wm.Conn().PollForEvent()
		if ev == nil && err == nil {
			return
		}
		if err != nil {
			t.Fatalf("X error: %v", err)
		}
		wm.handleEvent(ev)
	}
}

// checkTabs verifies that exactly the active tab of f is mapped.
func checkTabs(t *testing.T, f *Frame) {
	t.Helper()
	for i, win := range f.clients {
		attr, err := xproto.GetWindowAttributes(f.Conn(), win).Reply()
		if err != nil {
			t.Fatal(err)
		}
		mapped := attr.MapState != xproto.MapStateUnmapped
		if want := i == f.activeClient; mapped != want {
			t.Fatalf("tab %d (0x%x) mapped=%v, want %v (active tab %d)", i, win, mapped, want, f.activeClient)
		}
	}
}

func TestTabs(t *testing.T) {
	wm := newTestManager(t)
	f := wm.GetActiveFrame()

	var wins []xproto.Window
	for range 3 {
		wins = append(wins, newTestClient(t, wm))
	}
	drainEvents(t, wm)
	if len(f.clients) != 3 || f.activeClient != 2 {
		t.Fatalf("frame has %d tabs, active %d; want 3, active 2", len(f.clients), f.activeClient)
	}
	checkTabs(t, f)

	f.cycleClientRight()
	drainEvents(t, wm)
	if f.activeClient != 0 {
		t.Fatalf("active tab %d after cycling right from the last, want 0", f.activeClient)
	}
	checkTabs(t, f)

	f.cycleClientLeft()
	f.cycleClientLeft()
	drainEvents(t, wm)
	if f.activeClient != 1 {
		t.Fatalf("active tab %d after cycling left twice from 0, want 1", f.activeClient)
	}
	checkTabs(t, f)

	// hiding tabs must not be taken for the clients withdrawing
	if len(wm.Clients) != 3 {
		t.Fatalf("%d clients managed, want 3", len(wm.Clients))
	}

	// clicking the last tab selects it
	w := f.tabWidth()
	wm.clickTitleBar(f, int16(2*w+w/2))
	drainEvents(t, wm)
	if f.activeClient != 2 {
		t.Fatalf("active tab %d after clicking the third, want 2", f.activeClient)
	}
	checkTabs(t, f)

	// the active tab going away shows another one
	destroyTestClient(t, wins[2])
	drainEvents(t, wm)
	if len(f.clients) != 2 || f.GetActiveClient() == 0 {
		t.Fatalf("frame has %d tabs and active 0x%x after destroying the active one", len(f.clients), f.GetActiveClient())
	}
	checkTabs(t, f)

	// hidden tabs move along on a split, still hidden
	if err := wm.GetActiveWorkspace().splitH(); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	checkTabs(t, wm.GetActiveWorkspace().Root.children[0])
	if len(wm.Clients) != 2 {
		t.Fatalf("%d clients managed after the split, want 2", len(wm.Clients))
	}
}

func TestTabAt(t *testing.T) {
	wm := newTestManager(t)
	f := wm.GetActiveFrame()
	if f.tabAt(10) != -1 {
		t.Fatalf("tab found in an empty frame")
	}
	for range 4 {
		newTestClient(t, wm)
	}
	w := f.tabWidth()
	for x, want := range map[int16]int{0: 0, int16(w - 1): 0, int16(w): 1, int16(4*w - 1): 3, int16(4 * w): -1, -1: -1} {
		if got := f.tabAt(x); got != want {
			t.Errorf("tabAt(%d) = %d, want %d (tab width %d)", x, got, want, w)
		}
	}
}
