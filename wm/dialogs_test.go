package wm

import (
	"testing"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// newTestWindow creates a top-level window of its own connection, w×h,
// with set to set its properties before fion sees it.
func newTestWindow(t *testing.T, wm *Manager, w, h uint16, set func(conn *xgb.Conn, win xproto.Window)) xproto.Window {
	t.Helper()
	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	scr := wm.Screens[0].Info()
	win, _ := xproto.NewWindowId(conn)
	if err := xproto.CreateWindowChecked(conn, scr.RootDepth, win, scr.Root, 0, 0, w, h, 0,
		xproto.WindowClassInputOutput, scr.RootVisual, 0, nil).Check(); err != nil {
		t.Fatal(err)
	}
	if set != nil {
		set(conn, win)
	}
	conn.Sync()
	return win
}

func setWindowProp(conn *xgb.Conn, win xproto.Window, prop, typ xproto.Atom, v ...uint32) {
	buf := make([]byte, 4*len(v))
	for i, x := range v {
		buf[4*i], buf[4*i+1], buf[4*i+2], buf[4*i+3] = byte(x), byte(x>>8), byte(x>>16), byte(x>>24)
	}
	xproto.ChangeProperty(conn, xproto.PropModeReplace, win, prop, typ, 32, uint32(len(v)), buf)
}

func TestDialogs(t *testing.T) {
	wm := newTestManager(t)
	mod := wm.KeyboardManager.Mod
	s := wm.GetActiveScreen()
	parent := newTestClient(t, wm)
	drainEvents(t, wm)

	dlg := newTestWindow(t, wm, 200, 100, func(conn *xgb.Conn, win xproto.Window) {
		setWindowProp(conn, win, xproto.AtomWmTransientFor, xproto.AtomWindow, uint32(parent))
	})
	if wm.windowKind(dlg) != kindDialog {
		t.Fatalf("a transient window isn't taken for a dialog")
	}
	wm.manage(dlg, false)
	drainEvents(t, wm)
	if _, tab := wm.Clients[dlg]; tab || len(wm.dialogsOf(s.GetActiveWorkspace())) != 1 {
		t.Fatalf("the dialog was made a tab")
	}
	checkFocus(t, wm, dlg)

	// centered over its parent's frame
	g, err := xproto.GetGeometry(wm.Conn(), xproto.Drawable(dlg)).Reply()
	if err != nil {
		t.Fatal(err)
	}
	fx, fy := wm.Clients[parent].frame.origin()
	f := wm.Clients[parent].frame
	if cx, cy := int(g.X)+101, int(g.Y)+51; abs(int16(cx-int(fx)-int(f.g.W)/2)) > 2 || abs(int16(cy-int(fy)-int(f.g.H)/2)) > 2 {
		t.Fatalf("dialog at %d,%d, not centered on its frame %+v", g.X, g.Y, f.g)
	}

	// the keys go back to the frames, a click brings it back
	press(t, wm, XK_Tab, mod)
	checkFocus(t, wm, parent)
	wm.clientClicked(xproto.ButtonPressEvent{Event: dlg, Root: s.Info().Root})
	checkFocus(t, wm, dlg)

	// hidden with its workspace
	press(t, wm, XK_w, mod)
	drainEvents(t, wm)
	if attr, _ := xproto.GetWindowAttributes(wm.Conn(), dlg).Reply(); attr.MapState == xproto.MapStateViewable {
		t.Fatalf("the dialog shows on another workspace")
	}
	if _, ok := wm.dialogs[dlg]; !ok {
		t.Fatalf("the dialog was forgotten when hidden")
	}
	press(t, wm, XK_Prior, mod)
	drainEvents(t, wm)
	if attr, _ := xproto.GetWindowAttributes(wm.Conn(), dlg).Reply(); attr.MapState != xproto.MapStateViewable {
		t.Fatalf("the dialog didn't come back with its workspace")
	}

	// a dialog may size itself
	wm.handleConfigureRequest(xproto.ConfigureRequestEvent{Window: dlg,
		ValueMask: xproto.ConfigWindowWidth | xproto.ConfigWindowHeight, Width: 300, Height: 150})
	if g, _ := xproto.GetGeometry(wm.Conn(), xproto.Drawable(dlg)).Reply(); g.Width != 300 || g.Height != 150 || g.BorderWidth != 1 {
		t.Fatalf("dialog configured to %+v", g)
	}

	// gone, the focus goes back to the frames
	xproto.DestroyWindow(wm.Conn(), dlg)
	drainEvents(t, wm)
	if len(wm.dialogs) != 0 {
		t.Fatalf("the dialog destroyed is still managed")
	}
	checkFocus(t, wm, parent)
}

func TestDialogKinds(t *testing.T) {
	wm := newTestManager(t)
	a := wm.Screens[0].atoms
	fixed := newTestWindow(t, wm, 100, 100, func(conn *xgb.Conn, win xproto.Window) {
		// WM_NORMAL_HINTS with the same minimum and maximum
		setWindowProp(conn, win, xproto.AtomWmNormalHints, xproto.AtomWmSizeHints,
			1<<4|1<<5, 0, 0, 0, 0, 100, 100, 100, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	})
	typed := newTestWindow(t, wm, 100, 100, func(conn *xgb.Conn, win xproto.Window) {
		setWindowProp(conn, win, a.NET_WM_WINDOW_TYPE, xproto.AtomAtom, uint32(a.NET_WM_WINDOW_TYPE_DIALOG))
	})
	dock := newTestWindow(t, wm, 100, 20, func(conn *xgb.Conn, win xproto.Window) {
		setWindowProp(conn, win, a.NET_WM_WINDOW_TYPE, xproto.AtomAtom, uint32(a.NET_WM_WINDOW_TYPE_DOCK))
	})
	plain := newTestWindow(t, wm, 100, 100, nil)
	for _, c := range []struct {
		name string
		win  xproto.Window
		want windowKind
	}{{"fixed size", fixed, kindDialog}, {"dialog type", typed, kindDialog}, {"dock", dock, kindUnmanaged}, {"plain", plain, kindTab}} {
		if got := wm.windowKind(c.win); got != c.want {
			t.Errorf("%s window: kind %d, want %d", c.name, got, c.want)
		}
	}
}
