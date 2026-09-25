package wm

import (
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// newClosableClient creates a client, on its own connection, taking part in
// WM_DELETE_WINDOW when polite is set, and has fion manage it.
func newClosableClient(t *testing.T, wm *Manager, polite bool) (*xgb.Conn, xproto.Window) {
	t.Helper()
	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	scr := wm.GetActiveScreen()
	win, err := xproto.NewWindowId(conn)
	if err != nil {
		t.Fatal(err)
	}
	err = xproto.CreateWindowChecked(conn, scr.Info().RootDepth, win, scr.Info().Root, 0, 0, 100, 100, 0,
		xproto.WindowClassInputOutput, scr.Info().RootVisual, 0, nil).Check()
	if err != nil {
		t.Fatal(err)
	}
	if polite {
		data := make([]byte, 4)
		xgb.Put32(data, uint32(scr.atoms.WM_DELETE_WINDOW))
		err = xproto.ChangePropertyChecked(conn, xproto.PropModeReplace, win, scr.atoms.WM_PROTOCOLS,
			xproto.AtomAtom, 32, 1, data).Check()
		if err != nil {
			t.Fatal(err)
		}
	}
	wm.manageWindow(win, false)
	drainEvents(t, wm)
	return conn, win
}

// waitForgotten feeds fion the server's events until win is no longer
// managed.
func waitForgotten(t *testing.T, wm *Manager, win xproto.Window) {
	t.Helper()
	for range 100 {
		drainEvents(t, wm)
		if _, ok := wm.Clients[win]; !ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("0x%x still managed", win)
}

func TestCloseAsksFirst(t *testing.T) {
	wm := newTestManager(t)
	conn, win := newClosableClient(t, wm, true)

	// asked, as a close button does, and still there
	if err := wm.closeActive(); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	received := make(chan xproto.ClientMessageEvent, 1)
	go func() {
		for {
			ev, err := conn.WaitForEvent()
			if ev == nil && err == nil {
				return
			}
			if cm, ok := ev.(xproto.ClientMessageEvent); ok {
				received <- cm
				return
			}
		}
	}()
	atoms := wm.GetActiveScreen().atoms
	select {
	case cm := <-received:
		if cm.Type != atoms.WM_PROTOCOLS || xproto.Atom(cm.Data.Data32[0]) != atoms.WM_DELETE_WINDOW {
			t.Fatalf("client got %+v, want WM_DELETE_WINDOW", cm)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("client not asked to close")
	}
	if _, ok := wm.Clients[win]; !ok {
		t.Fatalf("client dropped after being asked to close")
	}

	// asked again: killed
	if err := wm.closeActive(); err != nil {
		t.Fatal(err)
	}
	waitForgotten(t, wm, win)
}

func TestCloseKillsWhenItCantAsk(t *testing.T) {
	wm := newTestManager(t)
	_, win := newClosableClient(t, wm, false)
	if err := wm.closeActive(); err != nil {
		t.Fatal(err)
	}
	waitForgotten(t, wm, win)
}
