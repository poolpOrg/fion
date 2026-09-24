package wm

import (
	"os"
	"testing"

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

	scr := ws.Screen.Info()
	win, err := xproto.NewWindowId(wm.Conn())
	if err != nil {
		t.Fatal(err)
	}
	xproto.CreateWindow(wm.Conn(), scr.RootDepth, win, scr.Root, 0, 0, 100, 100, 0,
		xproto.WindowClassInputOutput, scr.RootVisual, 0, nil)
	wm.manageWindow(win)

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
