package wm

import (
	"slices"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// TestAdoptExisting starts fion over windows already shown, as when it
// replaces another window manager or is restarted.
func TestAdoptExisting(t *testing.T) {
	useTestDisplay(t)
	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	scr := xproto.Setup(conn).DefaultScreen(conn)

	create := func(overrideRedirect bool) xproto.Window {
		t.Helper()
		win, err := xproto.NewWindowId(conn)
		if err != nil {
			t.Fatal(err)
		}
		or := uint32(0)
		if overrideRedirect {
			or = 1
		}
		err = xproto.CreateWindowChecked(conn, scr.RootDepth, win, scr.Root, 10, 10, 100, 100, 0,
			xproto.WindowClassInputOutput, scr.RootVisual, xproto.CwOverrideRedirect, []uint32{or}).Check()
		if err != nil {
			t.Fatal(err)
		}
		return win
	}
	// map until shown: the window manager of the previous test may still
	// be letting go of the root, and would swallow the request
	show := func(win xproto.Window) {
		t.Helper()
		for range 100 {
			xproto.MapWindow(conn, win)
			attr, err := xproto.GetWindowAttributes(conn, win).Reply()
			if err != nil {
				t.Fatal(err)
			}
			if attr.MapState == xproto.MapStateViewable {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("0x%x could not be shown", win)
	}

	bottom, top := create(false), create(false)
	show(bottom)
	show(top)
	hidden := create(false)
	popup := create(true)
	show(popup)

	wm, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(wm.Close)
	drainEvents(t, wm)

	f := wm.GetActiveFrame()
	if !slices.Equal(f.clients, []xproto.Window{bottom, top}) {
		t.Fatalf("frame holds %v, want the shown windows %v", f.clients, []xproto.Window{bottom, top})
	}
	if f.GetActiveClient() != top {
		t.Fatalf("active tab 0x%x, want the topmost window 0x%x", f.GetActiveClient(), top)
	}
	checkClientIn(t, wm, bottom, f)
	checkClientIn(t, wm, top, f)
	checkTabs(t, f)

	// adopting a shown window unmaps it: that is not a withdrawal
	if len(wm.Clients) != 2 {
		t.Fatalf("%d clients managed, want 2", len(wm.Clients))
	}
	for _, win := range []xproto.Window{hidden, popup} {
		if _, ok := wm.Clients[win]; ok {
			t.Fatalf("0x%x adopted, though unmapped or override-redirect", win)
		}
	}
}
