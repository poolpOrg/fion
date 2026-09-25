package wm

import (
	"fmt"
	"log"

	"github.com/jezek/xgb/xproto"
)

// The key bindings, all with Mod, grabbed on the root so that they reach
// fion wherever the pointer is:
//
//	Tab, Shift+Tab     next, previous tab
//	t                  new terminal tab
//	arrows             go to the frame on that side
//	Shift+arrows       split: new frame on that side
//	Page Down, Page Up next, previous workspace
//	w                  new workspace
//	f                  show the active tab full screen, and back
//	d                  close what has the focus, asking first
//	Return             launcher
//	space              scratchpad
//	s                  system panel
//	+, -               resize the active frame, growing, shrinking
//	m                  move the scratchpad
//	?                  cheat sheet
//	Escape             quit

const (
	XK_Next        xproto.Keysym = 0xFF56 // Page Down
	XK_Prior       xproto.Keysym = 0xFF55 // Page Up
	XK_ISO_LeftTab xproto.Keysym = 0xFE20 // Shift+Tab, on most keymaps
	XK_equal       xproto.Keysym = 0x003D
	XK_KP_Add      xproto.Keysym = 0xFFAB
	XK_KP_Subtract xproto.Keysym = 0xFFAD
)

type binding struct {
	sym      xproto.Keysym
	shift    bool
	ctrl     bool   // with Ctrl too, Alt when Mod is Ctrl
	anyShift bool   // for keys some layouts put on Shift, such as + and ?
	desc     string // for the cheat sheet
	do       func(wm *Manager) error
}

var bindings = []binding{
	{XK_Tab, false, false, false, "next tab", func(wm *Manager) error { wm.GetActiveFrame().cycleClientRight(); return nil }},
	{XK_Tab, true, false, false, "previous tab", func(wm *Manager) error { wm.GetActiveFrame().cycleClientLeft(); return nil }},
	{XK_t, false, false, false, "new terminal tab", func(wm *Manager) error { wm.spawnTerminal(); return nil }},

	{XK_Left, false, false, false, "go to the frame on the left", func(wm *Manager) error { return wm.focusFrame(dirLeft) }},
	{XK_Right, false, false, false, "go to the frame on the right", func(wm *Manager) error { return wm.focusFrame(dirRight) }},
	{XK_Up, false, false, false, "go to the frame above", func(wm *Manager) error { return wm.focusFrame(dirUp) }},
	{XK_Down, false, false, false, "go to the frame below", func(wm *Manager) error { return wm.focusFrame(dirDown) }},
	{XK_Left, true, false, false, "split: new frame on the left", func(wm *Manager) error { return wm.GetActiveFrame().splitTowards(dirLeft) }},
	{XK_Right, true, false, false, "split: new frame on the right", func(wm *Manager) error { return wm.GetActiveFrame().splitTowards(dirRight) }},
	{XK_Up, true, false, false, "split: new frame above", func(wm *Manager) error { return wm.GetActiveFrame().splitTowards(dirUp) }},
	{XK_Down, true, false, false, "split: new frame below", func(wm *Manager) error { return wm.GetActiveFrame().splitTowards(dirDown) }},

	{XK_Next, false, false, false, "next workspace", func(wm *Manager) error { wm.switchWorkspace(1); return nil }},
	{XK_Prior, false, false, false, "previous workspace", func(wm *Manager) error { wm.switchWorkspace(-1); return nil }},
	{XK_w, false, false, false, "new workspace", func(wm *Manager) error { return wm.createWorkspace() }},

	{XK_f, false, false, false, "show the tab full screen / back", func(wm *Manager) error { return wm.GetActiveScreen().toggleFullscreen() }},
	{XK_d, false, false, false, "close what has the focus, asking first", func(wm *Manager) error { return wm.requestClose() }},
	{XK_Return, false, false, false, "launcher", func(wm *Manager) error { return wm.openLauncher() }},
	{XK_Space, false, false, false, "show / hide the scratchpad", func(wm *Manager) error { return wm.GetActiveScreen().toggleScratchpad() }},
	{XK_s, false, false, false, "show / hide the system panel", func(wm *Manager) error { return wm.GetActiveScreen().togglePanel() }},

	{XK_Plus, false, false, true, "resize, growing: then arrows, Return", func(wm *Manager) error { return wm.startResize(true) }},
	{XK_Minus, false, false, true, "resize, shrinking: then arrows, Return", func(wm *Manager) error { return wm.startResize(false) }},
	{XK_Left, false, true, false, "move the tab to the frame on the left", func(wm *Manager) error { return wm.moveTab(dirLeft) }},
	{XK_Right, false, true, false, "move the tab to the frame on the right", func(wm *Manager) error { return wm.moveTab(dirRight) }},
	{XK_Up, false, true, false, "move the tab to the frame above", func(wm *Manager) error { return wm.moveTab(dirUp) }},
	{XK_Down, false, true, false, "move the tab to the frame below", func(wm *Manager) error { return wm.moveTab(dirDown) }},
	{XK_m, false, false, false, "move the scratchpad: then arrows, Return", func(wm *Manager) error { return wm.startMove() }},
}

// other keys for the same bindings
var keyAliases = map[xproto.Keysym]xproto.Keysym{
	XK_equal:       XK_Plus, // + without Shift, on many layouts
	XK_KP_Add:      XK_Plus,
	XK_KP_Subtract: XK_Minus,
	XK_ISO_LeftTab: XK_Tab,
}

// keyNames names the keys of the bindings, for the cheat sheet.
var keyNames = map[xproto.Keysym]string{
	XK_Tab: "Tab", XK_Left: "Left", XK_Right: "Right", XK_Up: "Up", XK_Down: "Down",
	XK_Next: "Page Down", XK_Prior: "Page Up", XK_Return: "Return", XK_Space: "space",
	XK_Escape: "Escape", XK_question: "?", XK_Plus: "+", XK_Minus: "-",
}

// keys names a binding's keys: Super+Shift+Tab, Super+Ctrl+Left.
func (b binding) keys(mod, ctrl string) string {
	name, ok := keyNames[b.sym]
	if !ok {
		name = string(rune(b.sym))
	}
	switch {
	case b.shift:
		return mod + "+Shift+" + name
	case b.ctrl:
		return mod + "+" + ctrl + "+" + name
	}
	return mod + "+" + name
}

// handleBinding runs the binding for a key pressed with Mod, and reports
// whether it was Mod+Escape, to quit.
func (wm *Manager) handleBinding(sym xproto.Keysym, shift, ctrl bool) bool {
	if alias, ok := keyAliases[sym]; ok {
		if sym == XK_ISO_LeftTab {
			shift = true
		}
		sym = alias
	}
	if sym == XK_Escape && !shift && !ctrl {
		return true
	}
	// wherever the layout puts it, with Shift or not
	if sym == XK_question && !ctrl {
		if err := wm.showCheatSheet(); err != nil {
			log.Printf("cheat sheet: %v", err)
		}
		return false
	}
	for _, b := range bindings {
		if b.sym == sym && b.ctrl == ctrl && (b.shift == shift || b.anyShift) {
			if sym != XK_f {
				wm.GetActiveScreen().leaveFullscreen()
			}
			// back to the frames, but to close the dialog
			if sym != XK_d {
				wm.dialogFocus = 0
			}
			if err := b.do(wm); err != nil {
				log.Printf("key 0x%x: %v", uint32(sym), err)
			}
			// whatever the binding made active
			wm.updateFocus()
			break
		}
	}
	return false
}

type direction int

const (
	dirLeft direction = iota
	dirRight
	dirUp
	dirDown
)

// frameTowards returns the leaf next to f on the side dir, in its
// workspace or, past its monitor's edge, in the workspace shown on the
// next monitor: among those touching that side, the one sharing the most
// of it, nil when there is none.
func (f *Frame) frameTowards(dir direction) *Frame {
	fx, fy := f.origin()
	x0, y0, x1, y1 := int(fx), int(fy), int(fx)+int(f.g.W), int(fy)+int(f.g.H)
	// the bar at the bottom of a monitor, and its panel, are no frames:
	// the frames above them reach the monitor below
	bottom := func(c *Frame, y1 int) int {
		if g := c.screen.Geometry(); !c.floating() && y1 == int(g.Y)+c.screen.workAreaH() {
			return int(g.Y) + int(g.H)
		}
		return y1
	}
	y1 = bottom(f, y1)

	var best *Frame
	bestShared := 0
	visit := func(c *Frame) {
		if !c.leaf || c == f {
			return
		}
		cx, cy := c.origin()
		a0, b0, a1, b1 := int(cx), int(cy), int(cx)+int(c.g.W), int(cy)+int(c.g.H)
		b1 = bottom(c, b1)
		var touches bool
		var shared int
		switch dir {
		case dirLeft:
			touches, shared = a1 == x0, min(y1, b1)-max(y0, b0)
		case dirRight:
			touches, shared = a0 == x1, min(y1, b1)-max(y0, b0)
		case dirUp:
			touches, shared = b1 == y0, min(x1, a1)-max(x0, a0)
		case dirDown:
			touches, shared = b0 == y1, min(x1, a1)-max(x0, a0)
		}
		if touches && shared > bestShared {
			best, bestShared = c, shared
		}
	}
	walk(f.workspace.Root, visit)
	if best == nil {
		for _, s := range f.wm().Screens {
			if s != f.screen {
				walk(s.GetActiveWorkspace().Root, visit)
			}
		}
	}
	return best
}

// focusFrame makes the frame on the side dir the active one.
func (wm *Manager) focusFrame(dir direction) error {
	f := wm.GetActiveFrame()
	if f.floating() {
		return fmt.Errorf("the scratchpad is shown")
	}
	if next := f.frameTowards(dir); next != nil {
		next.workspace.ActiveFrame = next
		if next.screen != f.screen {
			wm.setActiveScreen(next.screen)
		}
		next.screen.updateTitleBars()
		wm.updateFocus()
	}
	return nil
}

// splitTowards splits f, the new frame on the side dir.
func (f *Frame) splitTowards(dir direction) error {
	vertical := dir == dirLeft || dir == dirRight
	return f.split(vertical, dir == dirLeft || dir == dirUp)
}

// switchWorkspace shows the workspace delta away from the active one,
// wrapping around.
func (wm *Manager) switchWorkspace(delta int) {
	s := wm.GetActiveScreen()
	old := s.GetActiveWorkspace()
	var next *Workspace
	if delta > 0 {
		next = s.cycleWorkspaceRight()
	} else {
		next = s.cycleWorkspaceLeft()
	}
	if old != nil && old != next {
		next.Map()
		old.Unmap()
	}
}

// createWorkspace makes a new workspace and shows it.
func (wm *Manager) createWorkspace() error {
	s := wm.GetActiveScreen()
	old := s.GetActiveWorkspace()
	ws, err := s.newWorkspace()
	if err != nil {
		return err
	}
	ws.Map()
	old.Unmap()
	return nil
}

// moveTab moves the active tab to the frame on the side dir, on this
// monitor or the next, and follows it.
func (wm *Manager) moveTab(dir direction) error {
	f := wm.GetActiveFrame()
	if f.floating() {
		return fmt.Errorf("the scratchpad is shown")
	}
	win := f.GetActiveClient()
	if win == 0 {
		return fmt.Errorf("no tab to move")
	}
	target := f.frameTowards(dir)
	if target == nil {
		return nil
	}
	if err := wm.moveClient(win, target, -1); err != nil {
		return err
	}
	wm.setActiveScreen(target.screen)
	f.screen.updateTitleBars()
	return nil
}
