package wm

import (
	"github.com/jezek/xgb/xproto"
)

// The input focus follows the keyboard: it is the active tab of the
// active frame, of the monitor with the focus, a floating dialog when one
// was just opened or clicked, and a window of fion's that takes the keys
// when the frame is empty, so that they don't go to the window under the
// pointer. Clicking a window focuses it, the click reaching it too.

// focusTarget is the window that should have the focus, and whether it is
// a client.
func (wm *Manager) focusTarget() (xproto.Window, bool) {
	s := wm.GetActiveScreen()
	if s == nil {
		return wm.noFocus, false
	}
	if d := wm.focusedDialog(); d != 0 {
		return d, true
	}
	if s.fullscreen.client != 0 {
		return s.fullscreen.client, true
	}
	if c := wm.GetActiveFrame().GetActiveClient(); c != 0 {
		return c, true
	}
	return wm.noFocus, false
}

// updateFocus gives the input focus to focusTarget.
func (wm *Manager) updateFocus() {
	win, client := wm.focusTarget()
	if !client {
		xproto.SetInputFocus(wm.Conn(), xproto.InputFocusPointerRoot, win, xproto.TimeCurrentTime)
		wm.setActiveWindow(0)
		return
	}
	wm.focusClient(win)
	wm.setActiveWindow(win)
}

// focusClient gives win the focus as ICCCM has it: set, unless its
// WM_HINTS say it doesn't take input, and told with WM_TAKE_FOCUS when it
// asks to be.
func (wm *Manager) focusClient(win xproto.Window) {
	// X refuses the focus to a window not shown, as one being moved
	if attr, err := xproto.GetWindowAttributes(wm.Conn(), win).Reply(); err != nil || attr.MapState != xproto.MapStateViewable {
		return
	}
	if acceptsInput(wm, win) {
		xproto.SetInputFocus(wm.Conn(), xproto.InputFocusPointerRoot, win, xproto.TimeCurrentTime)
	}
	a := wm.Screens[0].atoms
	if wm.supportsProtocol(win, a.WM_TAKE_FOCUS) {
		ev := xproto.ClientMessageEvent{
			Format: 32,
			Window: win,
			Type:   a.WM_PROTOCOLS,
			Data: xproto.ClientMessageDataUnionData32New(
				[]uint32{uint32(a.WM_TAKE_FOCUS), xproto.TimeCurrentTime, 0, 0, 0}),
		}
		xproto.SendEvent(wm.Conn(), false, win, xproto.EventMaskNoEvent, string(ev.Bytes()))
	}
}

// acceptsInput reads the input field of win's WM_HINTS: true unless it says
// otherwise.
func acceptsInput(wm *Manager, win xproto.Window) bool {
	h, ok := readWMHints(wm, win)
	return !ok || h.flags&hintInput == 0 || h.input
}

const (
	hintInput   = 1 << 0
	hintUrgency = 1 << 8
)

type wmHints struct {
	flags uint32
	input bool
}

func readWMHints(wm *Manager, win xproto.Window) (wmHints, bool) {
	r, err := xproto.GetProperty(wm.Conn(), false, win, xproto.AtomWmHints, xproto.AtomWmHints, 0, 9).Reply()
	if err != nil || r.Format != 32 || r.ValueLen < 2 {
		return wmHints{}, false
	}
	v := func(i int) uint32 { return xgbGet32(r.Value[4*i:]) }
	return wmHints{flags: v(0), input: v(1) != 0}, true
}

func xgbGet32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// setActiveWindow tells EWMH clients, as pagers, which window has the
// focus.
func (wm *Manager) setActiveWindow(win xproto.Window) {
	if wm.activeWindow == win && wm.activeWindowSet {
		return
	}
	wm.activeWindow, wm.activeWindowSet = win, true
	s := wm.Screens[0]
	s.setProp32(s.Info().Root, s.atoms.NET_ACTIVE_WINDOW, xproto.AtomWindow, uint32(win))
}

// grabClicks has the clicks on a client reach fion first, to focus it.
func (wm *Manager) grabClicks(win xproto.Window) {
	xproto.GrabButton(wm.Conn(), false, win, xproto.EventMaskButtonPress,
		xproto.GrabModeSync, xproto.GrabModeAsync, xproto.WindowNone, xproto.CursorNone,
		xproto.ButtonIndexAny, xproto.ModMaskAny)
}

// clientClicked focuses the client clicked, its tab and frame made the
// active ones, and lets the click through.
func (wm *Manager) clientClicked(ev xproto.ButtonPressEvent) bool {
	win := ev.Event
	defer xproto.AllowEvents(wm.Conn(), xproto.AllowReplayPointer, ev.Time)
	if wm.dialogClicked(win) {
		return true
	}
	c, ok := wm.Clients[win]
	if !ok {
		return false
	}
	f := c.frame
	wm.setActiveScreen(f.screen)
	if !f.floating() {
		f.workspace.ActiveFrame = f
	}
	for i, cw := range f.clients {
		if cw == win && i != f.activeClient {
			f.selectClient(i)
		}
	}
	f.screen.updateTitleBars()
	wm.updateFocus()
	return true
}

// newNoFocusWindow makes the window that takes the keys when no client
// has the focus: out of sight, and mapped, as the focus must be.
func (wm *Manager) newNoFocusWindow(info xproto.ScreenInfo) error {
	w, err := xproto.NewWindowId(wm.Conn())
	if err != nil {
		return err
	}
	xproto.CreateWindow(wm.Conn(), 0, w, info.Root, -10, -10, 1, 1, 0,
		xproto.WindowClassInputOnly, info.RootVisual, xproto.CwOverrideRedirect|xproto.CwEventMask,
		[]uint32{1, xproto.EventMaskKeyPress})
	xproto.MapWindow(wm.Conn(), w)
	wm.noFocus = w
	return nil
}
