package wm

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestUrgency(t *testing.T) {
	wm := newTestManager(t)
	mod := wm.KeyboardManager.Mod
	s := wm.GetActiveScreen()
	bell := newTestClient(t, wm)
	if err := wm.createWorkspace(); err != nil {
		t.Fatal(err)
	}
	other := newTestClient(t, wm)
	drainEvents(t, wm)

	// the window on the first workspace rings
	setWindowProp(wm.Conn(), bell, xproto.AtomWmHints, xproto.AtomWmHints, hintUrgency, 0, 0, 0, 0, 0, 0, 0, 0)
	wm.urgencyChanged(bell)
	if !wm.Clients[bell].urgent || s.urgentSummary() != "! 1" {
		t.Fatalf("urgency not shown: %q", s.urgentSummary())
	}
	// the one with the focus can't be urgent
	setWindowProp(wm.Conn(), other, xproto.AtomWmHints, xproto.AtomWmHints, hintUrgency, 0, 0, 0, 0, 0, 0, 0, 0)
	wm.urgencyChanged(other)
	if wm.Clients[other].urgent {
		t.Fatalf("the focused window is marked urgent")
	}
	// which clears its hint, as xterm does
	setWindowProp(wm.Conn(), other, xproto.AtomWmHints, xproto.AtomWmHints, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	// going to it clears it
	press(t, wm, XK_Prior, mod)
	if wm.Clients[bell].urgent || s.urgentSummary() != "" {
		t.Fatalf("urgency kept once focused")
	}
	// as EWMH has it
	a := s.atoms
	wm.handleClientMessage(xproto.ClientMessageEvent{Format: 32, Window: other, Type: a.NET_WM_STATE,
		Data: xproto.ClientMessageDataUnionData32New([]uint32{1, uint32(a.NET_WM_STATE_DEMANDS_ATTENTION), 0, 0, 0})})
	if s.urgentSummary() != "! 2" {
		t.Fatalf("_NET_WM_STATE_DEMANDS_ATTENTION: %q", s.urgentSummary())
	}
	if before, _, _ := barPieces(barState{position: "[01:01/02]", urgent: "! 2", cpuPercent: -1, memPercent: -1, recording: -1}, 0); len(before) < 2 || before[1].s != "! 2" || !before[1].alert {
		t.Fatalf("bar pieces %+v", before)
	}
}
