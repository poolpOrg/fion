package wm

import (
	"slices"
	"testing"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func TestUTF8Titles(t *testing.T) {
	wm := newTestManager(t)
	name, utf8String := netWMName(wm.Conn())
	win := newTestWindow(t, wm, 100, 100, func(conn *xgb.Conn, win xproto.Window) {
		title := "café — ☕"
		xproto.ChangeProperty(conn, xproto.PropModeReplace, win, name, utf8String, 8, uint32(len(title)), []byte(title))
		xproto.ChangeProperty(conn, xproto.PropModeReplace, win, xproto.AtomWmName, xproto.AtomString, 8, 4, []byte("caf\xe9"))
	})
	if got := getWindowName(wm.Conn(), win); got != "café — ☕" {
		t.Fatalf("title %q, want the _NET_WM_NAME", got)
	}
	latin := newTestWindow(t, wm, 100, 100, func(conn *xgb.Conn, win xproto.Window) {
		xproto.ChangeProperty(conn, xproto.PropModeReplace, win, xproto.AtomWmName, xproto.AtomString, 8, 4, []byte("caf\xe9"))
	})
	if got := getWindowName(wm.Conn(), latin); got != "café" {
		t.Fatalf("Latin-1 title %q", got)
	}
	if c := char2b("é☕"); len(c) != 2 || c[1] != (xproto.Char2b{Byte1: 0x26, Byte2: 0x15}) {
		t.Fatalf("char2b = %v", c)
	}
	if got := latin1("é☕"); got != "\xe9?" {
		t.Fatalf("latin1 = %q", got)
	}
	if got := truncateRunes("é☕ab", 2); got != "é☕" {
		t.Fatalf("truncateRunes = %q", got)
	}
}

func TestEWMHFullscreenAndActivate(t *testing.T) {
	wm := newTestManager(t)
	s := wm.GetActiveScreen()
	a := s.atoms
	first := s.GetActiveWorkspace()
	win := newTestClient(t, wm)
	drainEvents(t, wm)

	// the client asks for full screen, then to leave it
	msg := func(typ xproto.Atom, data ...uint32) {
		wm.handleClientMessage(xproto.ClientMessageEvent{Format: 32, Window: win, Type: typ,
			Data: xproto.ClientMessageDataUnionData32New(append(data, make([]uint32, 5-len(data))...))})
		drainEvents(t, wm)
	}
	msg(a.NET_WM_STATE, 1, uint32(a.NET_WM_STATE_FULLSCREEN))
	if s.fullscreen.client != win || !wm.wantsFullscreen(win) {
		t.Fatalf("_NET_WM_STATE add didn't show the window full screen")
	}
	msg(a.NET_WM_STATE, 2, uint32(a.NET_WM_STATE_FULLSCREEN))
	if s.fullscreen.client != 0 || wm.wantsFullscreen(win) {
		t.Fatalf("_NET_WM_STATE toggle didn't put it back")
	}

	// activating it from another workspace shows its own
	if err := wm.createWorkspace(); err != nil {
		t.Fatal(err)
	}
	msg(a.NET_ACTIVE_WINDOW, 1)
	if s.GetActiveWorkspace() != first {
		t.Fatalf("_NET_ACTIVE_WINDOW didn't show the window's workspace")
	}
	checkFocus(t, wm, win)

	// listed
	r, err := xproto.GetProperty(wm.Conn(), false, s.Info().Root, a.NET_CLIENT_LIST, xproto.AtomWindow, 0, 64).Reply()
	if err != nil || r.ValueLen != 1 || xproto.Window(xgbGet32(r.Value)) != win {
		t.Fatalf("_NET_CLIENT_LIST = %v", r)
	}

	// a window asking for full screen before it is shown gets it
	full := newTestWindow(t, wm, 100, 100, func(conn *xgb.Conn, w xproto.Window) {
		setWindowProp(conn, w, a.NET_WM_STATE, xproto.AtomAtom, uint32(a.NET_WM_STATE_FULLSCREEN))
	})
	wm.manage(full, false)
	drainEvents(t, wm)
	if s.fullscreen.client != full {
		t.Fatalf("a window mapped full screen isn't shown so")
	}
	if !slices.Contains(s.supportedAtoms(), a.NET_WM_STATE_FULLSCREEN) {
		t.Fatalf("full screen not announced")
	}
}
