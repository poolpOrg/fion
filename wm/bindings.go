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
//	d                  close what has the focus, asking first
//	Return             launcher
//	space              scratchpad
//	s                  system panel
//	?                  cheat sheet
//	Escape             quit

const (
	XK_Next        xproto.Keysym = 0xFF56 // Page Down
	XK_Prior       xproto.Keysym = 0xFF55 // Page Up
	XK_ISO_LeftTab xproto.Keysym = 0xFE20 // Shift+Tab, on most keymaps
)

type binding struct {
	sym   xproto.Keysym
	shift bool
	desc  string // for the cheat sheet
	do    func(wm *Manager) error
}

var bindings = []binding{
	{XK_Tab, false, "next tab", func(wm *Manager) error { wm.GetActiveFrame().cycleClientRight(); return nil }},
	{XK_Tab, true, "previous tab", func(wm *Manager) error { wm.GetActiveFrame().cycleClientLeft(); return nil }},
	{XK_t, false, "new terminal tab", func(wm *Manager) error { wm.spawnTerminal(); return nil }},

	{XK_Left, false, "go to the frame on the left", func(wm *Manager) error { return wm.focusFrame(dirLeft) }},
	{XK_Right, false, "go to the frame on the right", func(wm *Manager) error { return wm.focusFrame(dirRight) }},
	{XK_Up, false, "go to the frame above", func(wm *Manager) error { return wm.focusFrame(dirUp) }},
	{XK_Down, false, "go to the frame below", func(wm *Manager) error { return wm.focusFrame(dirDown) }},
	{XK_Left, true, "split: new frame on the left", func(wm *Manager) error { return wm.GetActiveFrame().splitTowards(dirLeft) }},
	{XK_Right, true, "split: new frame on the right", func(wm *Manager) error { return wm.GetActiveFrame().splitTowards(dirRight) }},
	{XK_Up, true, "split: new frame above", func(wm *Manager) error { return wm.GetActiveFrame().splitTowards(dirUp) }},
	{XK_Down, true, "split: new frame below", func(wm *Manager) error { return wm.GetActiveFrame().splitTowards(dirDown) }},

	{XK_Next, false, "next workspace", func(wm *Manager) error { wm.switchWorkspace(1); return nil }},
	{XK_Prior, false, "previous workspace", func(wm *Manager) error { wm.switchWorkspace(-1); return nil }},
	{XK_w, false, "new workspace", func(wm *Manager) error { return wm.createWorkspace() }},

	{XK_d, false, "close what has the focus, asking first", func(wm *Manager) error { return wm.requestClose() }},
	{XK_Return, false, "launcher", func(wm *Manager) error { return wm.openLauncher() }},
	{XK_Space, false, "show / hide the scratchpad", func(wm *Manager) error { return wm.GetActiveScreen().toggleScratchpad() }},
	{XK_s, false, "show / hide the system panel", func(wm *Manager) error { return wm.GetActiveScreen().togglePanel() }},
}

// keyNames names the keys of the bindings, for the cheat sheet.
var keyNames = map[xproto.Keysym]string{
	XK_Tab: "Tab", XK_Left: "Left", XK_Right: "Right", XK_Up: "Up", XK_Down: "Down",
	XK_Next: "Page Down", XK_Prior: "Page Up", XK_Return: "Return", XK_Space: "space",
	XK_Escape: "Escape", XK_question: "?",
}

// keys names a binding's keys: Super+Shift+Tab.
func (b binding) keys(mod string) string {
	name, ok := keyNames[b.sym]
	if !ok {
		name = string(rune(b.sym))
	}
	if b.shift {
		return mod + "+Shift+" + name
	}
	return mod + "+" + name
}

// handleBinding runs the binding for a key pressed with Mod, and reports
// whether it was Mod+Escape, to quit.
func (wm *Manager) handleBinding(sym xproto.Keysym, shift bool) bool {
	if sym == XK_ISO_LeftTab {
		sym, shift = XK_Tab, true
	}
	if sym == XK_Escape && !shift {
		return true
	}
	// wherever the layout puts it, with Shift or not
	if sym == XK_question {
		if err := wm.showCheatSheet(); err != nil {
			log.Printf("cheat sheet: %v", err)
		}
		return false
	}
	for _, b := range bindings {
		if b.sym == sym && b.shift == shift {
			if err := b.do(wm); err != nil {
				log.Printf("key 0x%x: %v", uint32(sym), err)
			}
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
// workspace: among those touching that side, the one sharing the most of
// it, nil when there is none.
func (f *Frame) frameTowards(dir direction) *Frame {
	fx, fy := f.origin()
	x0, y0, x1, y1 := int(fx), int(fy), int(fx)+int(f.g.W), int(fy)+int(f.g.H)

	var best *Frame
	bestShared := 0
	walk(f.workspace.Root, func(c *Frame) {
		if !c.leaf || c == f {
			return
		}
		cx, cy := c.origin()
		a0, b0, a1, b1 := int(cx), int(cy), int(cx)+int(c.g.W), int(cy)+int(c.g.H)
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
	})
	return best
}

// focusFrame makes the frame on the side dir the active one.
func (wm *Manager) focusFrame(dir direction) error {
	f := wm.GetActiveFrame()
	if f.floating() {
		return fmt.Errorf("the scratchpad is shown")
	}
	if next := f.frameTowards(dir); next != nil {
		f.workspace.ActiveFrame = next
		f.screen.updateTitleBars()
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
