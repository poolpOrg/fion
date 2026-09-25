package wm

import (
	"slices"
	"testing"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

// checkClientIn verifies that fion and the X server agree win is in f.
func checkClientIn(t *testing.T, wm *Manager, win xproto.Window, f *Frame) {
	t.Helper()
	if c := wm.Clients[win]; c == nil || c.frame != f {
		t.Fatalf("0x%x is not tracked in frame 0x%x", win, f.window)
	}
	if !slices.Contains(f.clients, win) {
		t.Fatalf("0x%x is not a tab of frame 0x%x", win, f.window)
	}
	tree, err := xproto.QueryTree(wm.Conn(), win).Reply()
	if err != nil {
		t.Fatal(err)
	}
	if tree.Parent != f.window {
		t.Fatalf("0x%x is a child of 0x%x, want frame 0x%x", win, tree.Parent, f.window)
	}
}

// splitWithClients splits the root frame left / right and opens n clients
// in the left one.
func splitWithClients(t *testing.T, wm *Manager, n int) (left, right *Frame, wins []xproto.Window) {
	t.Helper()
	ws := wm.GetActiveWorkspace()
	if err := ws.splitV(); err != nil {
		t.Fatal(err)
	}
	left, right = ws.Root.children[0], ws.Root.children[1]
	ws.ActiveFrame = left
	for range n {
		wins = append(wins, newTestClient(t, wm))
	}
	drainEvents(t, wm)
	return left, right, wins
}

func TestMoveClient(t *testing.T) {
	wm := newTestManager(t)
	left, right, wins := splitWithClients(t, wm, 3)

	// to another frame, which becomes active
	if err := wm.moveClient(wins[1], right, -1); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	checkClientIn(t, wm, wins[1], right)
	checkTabs(t, left)
	checkTabs(t, right)
	if right.GetActiveClient() != wins[1] || !right.isActive() {
		t.Fatalf("moved client is not the active tab of the active frame")
	}
	if !slices.Equal(left.clients, []xproto.Window{wins[0], wins[2]}) {
		t.Fatalf("left tabs %v after the move", left.clients)
	}

	// within the same frame, to the front
	if err := wm.moveClient(wins[2], left, 0); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	if !slices.Equal(left.clients, []xproto.Window{wins[2], wins[0]}) || left.activeClient != 0 {
		t.Fatalf("left tabs %v, active %d after reordering", left.clients, left.activeClient)
	}
	checkTabs(t, left)

	// into the scratchpad and back
	s := wm.GetActiveScreen()
	if err := s.toggleScratchpad(); err != nil {
		t.Fatal(err)
	}
	if err := wm.moveClient(wins[0], s.scratchpad, -1); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	checkClientIn(t, wm, wins[0], s.scratchpad)
	checkTabs(t, s.scratchpad)
	checkTabs(t, left)
	checkFocus(t, wm, wins[0])

	if err := wm.moveClient(wins[0], left, -1); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	checkClientIn(t, wm, wins[0], left)
	checkTabs(t, left)
	if len(s.scratchpad.clients) != 0 {
		t.Fatalf("scratchpad still holds %d clients", len(s.scratchpad.clients))
	}

	// no move is taken for a client withdrawing
	if len(wm.Clients) != 3 {
		t.Fatalf("%d clients managed, want 3", len(wm.Clients))
	}
	checkTree(t, wm.GetActiveWorkspace())

	if err := wm.moveClient(wins[0], wm.GetActiveWorkspace().Root, -1); err == nil {
		t.Fatalf("moved a client to a split frame")
	}
}

func TestFrameAt(t *testing.T) {
	wm := newTestManager(t)
	left, right, _ := splitWithClients(t, wm, 0)
	sg := wm.GetActiveScreen().Geometry()

	for _, tc := range []struct {
		x, y   int16
		want   *Frame
		fx, fy int16
	}{
		{10, 30, left, 10, 30},
		{int16(sg.W) - 10, 5, right, int16(right.g.W) - 10, 5},
		{10, int16(sg.H) - 5, nil, 0, 0}, // the info bar
	} {
		f, fx, fy := wm.frameAt(tc.x, tc.y)
		if f != tc.want || (f != nil && (fx != tc.fx || fy != tc.fy)) {
			t.Errorf("frameAt(%d, %d) = 0x%p (%d, %d), want 0x%p (%d, %d)", tc.x, tc.y, f, fx, fy, tc.want, tc.fx, tc.fy)
		}
	}

	// the shown scratchpad is on top
	s := wm.GetActiveScreen()
	if err := s.toggleScratchpad(); err != nil {
		t.Fatal(err)
	}
	ox, oy := s.scratchpad.origin()
	if f, fx, fy := wm.frameAt(ox+5, oy+5); f != s.scratchpad || fx != 5 || fy != 5 {
		t.Fatalf("frameAt the scratchpad's corner = 0x%p (%d, %d)", f, fx, fy)
	}
}

// TestDragTab drags a tab with XTEST, as a user would.
func TestDragTab(t *testing.T) {
	wm := newTestManager(t)
	left, right, wins := splitWithClients(t, wm, 2)

	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := xtest.Init(conn); err != nil {
		t.Skipf("XTEST: %v", err)
	}
	root := wm.GetActiveScreen().Info().Root
	fake := func(kind byte, detail byte, x, y int16) {
		t.Helper()
		if err := xtest.FakeInputChecked(conn, kind, detail, 0, root, x, y, 0).Check(); err != nil {
			t.Fatal(err)
		}
		drainEvents(t, wm)
	}

	// the first tab of the left frame, dropped in the right frame
	lx, ly := left.origin()
	fake(xproto.MotionNotify, 0, lx+10, ly+10)
	fake(xproto.ButtonPress, 1, lx+10, ly+10)
	if wm.drag == nil {
		t.Fatalf("pressing a tab didn't start following the pointer")
	}
	rx, ry := right.origin()
	fake(xproto.MotionNotify, 0, rx+100, ry+100)
	if wm.drag == nil || wm.drag.window == 0 {
		t.Fatalf("moving the pointer didn't start the drag")
	}
	fake(xproto.ButtonRelease, 1, rx+100, ry+100)
	if wm.drag != nil {
		t.Fatalf("the drag didn't end on release")
	}

	checkClientIn(t, wm, wins[0], right)
	checkClientIn(t, wm, wins[1], left)
	checkTabs(t, left)
	checkTabs(t, right)

	// a press without moving is a click: nothing moves
	fake(xproto.MotionNotify, 0, lx+10, ly+10)
	fake(xproto.ButtonPress, 1, lx+10, ly+10)
	fake(xproto.ButtonRelease, 1, lx+10, ly+10)
	checkClientIn(t, wm, wins[1], left)
	if len(wm.Clients) != 2 {
		t.Fatalf("%d clients managed, want 2", len(wm.Clients))
	}
}
