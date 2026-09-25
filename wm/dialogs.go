package wm

import (
	"log"
	"slices"

	"github.com/jezek/xgb/xproto"
)

// Dialogs, and the other windows that don't make sense as tabs, float
// above the frames instead: centered over the window they belong to, or
// the monitor, with the focus when they open, and shown with their
// workspace. Docks, desktops and notifications are shown as they ask,
// without being managed.

type dialog struct {
	win xproto.Window
	ws  *Workspace // shown with it

	ignoreUnmap    int
	closeRequested bool
}

// windowKind tells what to do with a window about to be shown.
type windowKind int

const (
	kindTab windowKind = iota
	kindDialog
	kindUnmanaged
)

func (wm *Manager) windowKind(win xproto.Window) windowKind {
	a := wm.Screens[0].atoms
	for _, t := range wm.atomList(win, a.NET_WM_WINDOW_TYPE) {
		switch t {
		case a.NET_WM_WINDOW_TYPE_DOCK, a.NET_WM_WINDOW_TYPE_DESKTOP, a.NET_WM_WINDOW_TYPE_NOTIFICATION:
			return kindUnmanaged
		case a.NET_WM_WINDOW_TYPE_DIALOG, a.NET_WM_WINDOW_TYPE_UTILITY,
			a.NET_WM_WINDOW_TYPE_SPLASH, a.NET_WM_WINDOW_TYPE_TOOLBAR:
			return kindDialog
		}
	}
	if wm.transientFor(win) != 0 || fixedSize(wm, win) {
		return kindDialog
	}
	return kindTab
}

// atomList reads a window's property holding atoms.
func (wm *Manager) atomList(win xproto.Window, prop xproto.Atom) []xproto.Atom {
	r, err := xproto.GetProperty(wm.Conn(), false, win, prop, xproto.AtomAtom, 0, 32).Reply()
	if err != nil || r.Format != 32 {
		return nil
	}
	out := make([]xproto.Atom, r.ValueLen)
	for i := range out {
		out[i] = xproto.Atom(xgbGet32(r.Value[4*i:]))
	}
	return out
}

// transientFor is the window win is a dialog of, 0 when none.
func (wm *Manager) transientFor(win xproto.Window) xproto.Window {
	r, err := xproto.GetProperty(wm.Conn(), false, win, xproto.AtomWmTransientFor, xproto.AtomWindow, 0, 1).Reply()
	if err != nil || r.Format != 32 || r.ValueLen < 1 {
		return 0
	}
	return xproto.Window(xgbGet32(r.Value))
}

// fixedSize reports whether win's WM_NORMAL_HINTS give it one size, as
// most dialogs have.
func fixedSize(wm *Manager, win xproto.Window) bool {
	r, err := xproto.GetProperty(wm.Conn(), false, win, xproto.AtomWmNormalHints, xproto.AtomWmSizeHints, 0, 18).Reply()
	if err != nil || r.Format != 32 || r.ValueLen < 9 {
		return false
	}
	v := func(i int) uint32 { return xgbGet32(r.Value[4*i:]) }
	const pMinSize, pMaxSize = 1 << 4, 1 << 5
	flags := v(0)
	return flags&pMinSize != 0 && flags&pMaxSize != 0 && v(5) == v(7) && v(6) == v(8) && v(5) > 0
}

// manageDialog floats win above the frames, centered over the window it
// belongs to, or the active monitor, and focuses it.
func (wm *Manager) manageDialog(win xproto.Window, mapped bool) {
	if _, ok := wm.dialogs[win]; ok {
		return
	}
	s := wm.GetActiveScreen()
	ws := s.GetActiveWorkspace()
	area := s.Geometry()
	area.H = uint16(s.workAreaH())
	if owner, ok := wm.Clients[wm.transientFor(win)]; ok {
		f := owner.frame
		if !f.floating() {
			ws = f.workspace
		}
		x, y := f.origin()
		area = Geometry{x, y, f.g.W, f.g.H}
		if area.W < 200 || area.H < 150 {
			area = f.screen.Geometry()
		}
	}

	w, h := uint16(320), uint16(200)
	if g, err := xproto.GetGeometry(wm.Conn(), xproto.Drawable(win)).Reply(); err == nil {
		w, h = g.Width, g.Height
	}
	m := ws.Screen.Geometry()
	w, h = min(w, m.W-2), min(h, uint16(ws.Screen.workAreaH())-2)
	x := int(area.X) + (int(area.W)-int(w)-2)/2
	y := int(area.Y) + (int(area.H)-int(h)-2)/2
	// on its monitor
	x = max(int(m.X), min(x, int(m.X)+int(m.W)-int(w)-2))
	y = max(int(m.Y), min(y, int(m.Y)+ws.Screen.workAreaH()-int(h)-2))

	conn := wm.Conn()
	// the values in the order of the mask's bits
	xproto.ChangeWindowAttributes(conn, win, xproto.CwEventMask|xproto.CwBorderPixel,
		[]uint32{colorAccent, xproto.EventMaskStructureNotify | xproto.EventMaskPropertyChange})
	xproto.ChangeSaveSet(conn, xproto.SetModeInsert, win)
	xproto.ConfigureWindow(conn, win,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight|xproto.ConfigWindowBorderWidth|xproto.ConfigWindowStackMode,
		[]uint32{uint32(x), uint32(y), uint32(w), uint32(h), 1, xproto.StackModeAbove})
	d := &dialog{win: win, ws: ws}
	wm.dialogs[win] = d
	wm.grabClicks(win)
	if ws == ws.Screen.GetActiveWorkspace() {
		xproto.MapWindow(conn, win)
		wm.dialogFocus = win
	}
	log.Printf("managing 0x%x, floating", win)
	wm.updateClientList()
	wm.updateFocus()
}

// focusedDialog is the floating dialog with the focus, 0 when none.
func (wm *Manager) focusedDialog() xproto.Window {
	d, ok := wm.dialogs[wm.dialogFocus]
	if !ok || d.ws != d.ws.Screen.GetActiveWorkspace() || d.ws.Screen != wm.GetActiveScreen() {
		return 0
	}
	return d.win
}

// dialogClicked focuses the dialog win, and reports whether it is one.
func (wm *Manager) dialogClicked(win xproto.Window) bool {
	d, ok := wm.dialogs[win]
	if !ok {
		return false
	}
	wm.setActiveScreen(d.ws.Screen)
	wm.dialogFocus = win
	xproto.ConfigureWindow(wm.Conn(), win, xproto.ConfigWindowStackMode, []uint32{xproto.StackModeAbove})
	wm.updateFocus()
	return true
}

// forgetDialog drops a dialog gone, the focus going to another one of
// its workspace, or back to the frames.
func (wm *Manager) forgetDialog(win xproto.Window) bool {
	d, ok := wm.dialogs[win]
	if !ok {
		return false
	}
	delete(wm.dialogs, win)
	wm.updateClientList()
	if wm.dialogFocus == win {
		wm.dialogFocus = 0
		for _, o := range wm.dialogs {
			if o.ws == d.ws {
				wm.dialogFocus = o.win
			}
		}
	}
	wm.updateFocus()
	return true
}

// showDialogs maps, or unmaps, the dialogs of ws, as it is shown or
// hidden, keeping them above.
func (wm *Manager) showDialogs(ws *Workspace, shown bool) {
	for _, d := range wm.dialogs {
		if d.ws != ws {
			continue
		}
		if shown {
			xproto.MapWindow(wm.Conn(), d.win)
			xproto.ConfigureWindow(wm.Conn(), d.win, xproto.ConfigWindowStackMode, []uint32{xproto.StackModeAbove})
		} else {
			d.ignoreUnmap++
			xproto.UnmapWindow(wm.Conn(), d.win)
		}
	}
}

// moveDialogs gives the dialogs of a workspace removed to another.
func (wm *Manager) moveDialogs(from, to *Workspace) {
	for _, d := range wm.dialogs {
		if d.ws == from {
			d.ws = to
		}
	}
}

// closeDialog asks the dialog to close, and kills it when asked again.
func (wm *Manager) closeDialog(win xproto.Window) {
	d, ok := wm.dialogs[win]
	if !ok {
		return
	}
	a := wm.Screens[0].atoms
	if !d.closeRequested && wm.supportsProtocol(win, a.WM_DELETE_WINDOW) {
		d.closeRequested = true
		wm.sendDelete(win)
		return
	}
	xproto.KillClient(wm.Conn(), uint32(win))
}

// dialogsOf lists the dialogs shown with ws, for tests and closing.
func (wm *Manager) dialogsOf(ws *Workspace) []xproto.Window {
	var out []xproto.Window
	for _, d := range wm.dialogs {
		if d.ws == ws {
			out = append(out, d.win)
		}
	}
	slices.Sort(out)
	return out
}

// handleConfigureRequest does what a window not managed asks, lets a
// dialog size and place itself, and tells a tab where it is, as it can't
// move.
func (wm *Manager) handleConfigureRequest(ev xproto.ConfigureRequestEvent) {
	conn := wm.Conn()
	if c, ok := wm.Clients[ev.Window]; ok {
		wm.sendConfigureNotify(ev.Window, c)
		return
	}
	mask := ev.ValueMask &^ xproto.ConfigWindowSibling
	if _, ok := wm.dialogs[ev.Window]; ok {
		// a border of fion's, and above the frames
		mask &^= xproto.ConfigWindowBorderWidth | xproto.ConfigWindowStackMode
	}
	var values []uint32
	for _, f := range []struct {
		bit uint16
		v   uint32
	}{
		{xproto.ConfigWindowX, uint32(ev.X)}, {xproto.ConfigWindowY, uint32(ev.Y)},
		{xproto.ConfigWindowWidth, uint32(ev.Width)}, {xproto.ConfigWindowHeight, uint32(ev.Height)},
		{xproto.ConfigWindowBorderWidth, uint32(ev.BorderWidth)},
		{xproto.ConfigWindowStackMode, uint32(ev.StackMode)},
	} {
		if mask&f.bit != 0 {
			values = append(values, f.v)
		}
	}
	if len(values) > 0 {
		xproto.ConfigureWindow(conn, ev.Window, mask, values)
	}
}

// sendConfigureNotify tells a tab its place on the root, as ICCCM asks when
// the window manager doesn't grant a request.
func (wm *Manager) sendConfigureNotify(win xproto.Window, c *Client) {
	g, err := xproto.GetGeometry(wm.Conn(), xproto.Drawable(win)).Reply()
	if err != nil {
		return
	}
	x, y := c.frame.origin()
	if full := c.frame.screen.fullscreen; full.client == win {
		sg := c.frame.screen.Geometry()
		x, y = sg.X-g.X, sg.Y-g.Y
	}
	ev := xproto.ConfigureNotifyEvent{
		Event: win, Window: win, AboveSibling: xproto.WindowNone,
		X: x + g.X, Y: y + g.Y, Width: g.Width, Height: g.Height, BorderWidth: g.BorderWidth,
	}
	xproto.SendEvent(wm.Conn(), false, win, xproto.EventMaskStructureNotify, string(ev.Bytes()))
}
