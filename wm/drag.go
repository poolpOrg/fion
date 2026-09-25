package wm

import (
	"fmt"
	"log"
	"slices"

	"github.com/jezek/xgb/xproto"
)

// Tabs are dragged with the first button, as in ion: dropped on another
// frame, the shown scratchpad included, a tab moves its client there;
// dropped elsewhere in its own title bar, it moves to that place.

// how far the pointer must move before a press on a tab becomes a drag
const dragThreshold = 4

type tabDrag struct {
	frame          *Frame
	client         xproto.Window
	startX, startY int16

	// follows the pointer once the drag started, 0 until then
	window xproto.Window
	w      uint16
}

// beginTabDrag starts following the pointer after a press on a tab.
func (wm *Manager) beginTabDrag(f *Frame, ev xproto.ButtonPressEvent) {
	i := f.tabAt(ev.EventX)
	if i < 0 {
		return
	}
	r, err := xproto.GrabPointer(wm.Conn(), false, f.screen.Info().Root,
		xproto.EventMaskButtonRelease|xproto.EventMaskPointerMotion,
		xproto.GrabModeAsync, xproto.GrabModeAsync,
		xproto.WindowNone, xproto.CursorNone, ev.Time).Reply()
	if err != nil || r.Status != xproto.GrabStatusSuccess {
		log.Printf("grab pointer: %v", err)
		return
	}
	wm.drag = &tabDrag{
		frame:  f,
		client: f.clients[i],
		startX: ev.RootX,
		startY: ev.RootY,
		w:      uint16(min(f.tabWidth(), scaled(200))),
	}
}

func (wm *Manager) dragMotion(ev xproto.MotionNotifyEvent) {
	d := wm.drag
	if d == nil {
		return
	}
	if d.window == 0 {
		if abs(ev.RootX-d.startX) < dragThreshold && abs(ev.RootY-d.startY) < dragThreshold {
			return
		}
		if err := wm.createDragWindow(d); err != nil {
			log.Printf("drag: %v", err)
			return
		}
	}
	xproto.ConfigureWindow(wm.Conn(), d.window, xproto.ConfigWindowX|xproto.ConfigWindowY,
		[]uint32{uint32(ev.RootX + 8), uint32(ev.RootY + 8)})
}

// createDragWindow creates the tab that follows the pointer.
func (wm *Manager) createDragWindow(d *tabDrag) error {
	scr := d.frame.screen.Info()
	w, err := xproto.NewWindowId(wm.Conn())
	if err != nil {
		return err
	}
	xproto.CreateWindow(wm.Conn(), scr.RootDepth, w, scr.Root,
		d.startX+8, d.startY+8, d.w, uint16(titleH()-2), 1,
		xproto.WindowClassInputOutput, scr.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask,
		[]uint32{colorAccent, colorBorder, xproto.EventMaskExposure})
	xproto.MapWindow(wm.Conn(), w)
	d.window = w
	return nil
}

// drawDragWindow draws the dragged tab's title, on Expose.
func (wm *Manager) drawDragWindow() {
	d := wm.drag
	title := getWindowName(wm.Conn(), d.client)
	if n := (int(d.w) - 8) / charW(); len(title) > n {
		title = title[:max(n, 0)]
	}
	gc := d.frame.barGC
	xproto.ChangeGC(wm.Conn(), gc, xproto.GcForeground|xproto.GcBackground,
		[]uint32{colorAccentText, colorAccent})
	xproto.ImageText8(wm.Conn(), byte(len(title)), xproto.Drawable(d.window), gc, 4, int16(baseline(titleH()-2)), title)
}

// endTabDrag drops the dragged tab where the button was released.
func (wm *Manager) endTabDrag(ev xproto.ButtonReleaseEvent) {
	d := wm.drag
	if d == nil {
		return
	}
	wm.drag = nil
	xproto.UngrabPointer(wm.Conn(), ev.Time)
	if d.window == 0 {
		return // a click, handled on the press
	}
	xproto.DestroyWindow(wm.Conn(), d.window)

	target, x, y := wm.frameAt(ev.RootX, ev.RootY)
	if target == nil {
		return
	}
	// dropped on a title bar, the tab goes where it was dropped
	index := -1
	if int(y) < titleH() {
		index = target.tabAt(x)
	}
	if err := wm.moveClient(d.client, target, index); err != nil {
		log.Printf("drop: %v", err)
	}
}

// origin is the position of f's inside in root coordinates.
func (f *Frame) origin() (int16, int16) {
	x, y := f.g.X, f.g.Y
	if f.floating() {
		x, y = x+1, y+1 // its border
	} else {
		// in its workspace, on its monitor
		g := f.screen.Geometry()
		x, y = x+g.X, y+g.Y
	}
	for p := f.parent; p != nil; p = p.parent {
		x, y = x+p.g.X, y+p.g.Y
	}
	return x, y
}

// frameAt returns the visible leaf frame at root coordinates x, y, and the
// point in the frame's coordinates.
func (wm *Manager) frameAt(x, y int16) (*Frame, int16, int16) {
	inside := func(f *Frame) (bool, int16, int16) {
		ox, oy := f.origin()
		fx, fy := x-ox, y-oy
		return fx >= 0 && fy >= 0 && int(fx) < int(f.g.W) && int(fy) < int(f.g.H), fx, fy
	}

	for _, s := range wm.Screens {
		if s.scratchpadShown {
			if ok, fx, fy := inside(s.scratchpad); ok {
				return s.scratchpad, fx, fy
			}
		}
	}
	var found *Frame
	var fx, fy int16
	for _, s := range wm.Screens {
		walk(s.GetActiveWorkspace().Root, func(f *Frame) {
			if !f.leaf || found != nil {
				return
			}
			if ok, x, y := inside(f); ok {
				found, fx, fy = f, x, y
			}
		})
	}
	return found, fx, fy
}

// moveClient moves a client to the leaf target, as its active tab at index
// among its tabs, or last when index is out of range.
func (wm *Manager) moveClient(win xproto.Window, target *Frame, index int) error {
	c, ok := wm.Clients[win]
	if !ok {
		return fmt.Errorf("0x%x is not managed", win)
	}
	if !target.leaf {
		return fmt.Errorf("frame is split")
	}

	source := c.frame
	source.RemoveClient(win)
	if source != target {
		// reparenting a mapped window unmaps it first
		if c.mapped {
			c.ignoreUnmap++
		}
		xproto.ReparentWindow(wm.Conn(), win, target.window, 0, int16(titleH()))
		c.frame = target
	}

	if index < 0 || index > len(target.clients) {
		index = len(target.clients)
	}
	target.clients = slices.Insert(target.clients, index, win)
	target.layout()
	target.selectClient(index)
	if source != target {
		source.showActiveClient()
	}

	if !target.floating() {
		target.workspace.ActiveFrame = target
	}
	target.screen.updateTitleBars()
	wm.updateFocus()
	return nil
}

func abs(v int16) int16 {
	if v < 0 {
		return -v
	}
	return v
}
