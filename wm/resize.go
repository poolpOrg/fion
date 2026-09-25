package wm

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jezek/xgb/xproto"
)

// Mod++ and Mod+- resize the active frame, growing and shrinking it: the
// arrows then move its edge on that side, outward or inward, Return
// confirms and Escape undoes. Mod+m moves the scratchpad the same way. The
// scratchpad's place and size are kept across restarts.

// the smallest a frame is resized to, along either axis, and how far a
// key moves an edge
func minFrameSize() int { return 3 * titleH() }
func resizeStep() int   { return scaled(24) }

// keyMode takes the keys pressed until it is done: its key reports so.
type keyMode struct {
	key func(sym xproto.Keysym) (done bool)
}

// arrowDirection tells the direction an arrow key points to.
func arrowDirection(sym xproto.Keysym) (direction, bool) {
	switch sym {
	case XK_Left:
		return dirLeft, true
	case XK_Right:
		return dirRight, true
	case XK_Up:
		return dirUp, true
	case XK_Down:
		return dirDown, true
	}
	return 0, false
}

// edgeTowards returns the split frame whose border is f's edge on the side
// dir, and whether f is in its first child: nil at the screen's edge.
func (f *Frame) edgeTowards(dir direction) (*Frame, bool) {
	vertical := dir == dirLeft || dir == dirRight
	wantFirst := dir == dirRight || dir == dirDown
	child := f
	for p := f.parent; p != nil; child, p = p, p.parent {
		if first := p.children[0] == child; p.vertical == vertical && first == wantFirst {
			return p, first
		}
	}
	return nil, false
}

// resize moves f's edge on the side dir by delta pixels, outward, or
// inward when negative. It reports whether there was an edge to move.
func (f *Frame) resize(dir direction, delta int) bool {
	if f.floating() {
		f.moveEdges(dir, delta, false)
		return true
	}
	p, first := f.edgeTowards(dir)
	if p == nil {
		return false
	}
	total := int(p.g.H)
	if p.vertical {
		total = int(p.g.W)
	}
	size := int(p.firstShare(uint16(total)))
	if first {
		size += delta
	} else {
		size -= delta
	}
	size = max(minFrameSize(), min(size, total-minFrameSize()))
	// half a pixel over, so that rounding down gives size back
	p.ratio = (float64(size) + 0.5) / float64(total)
	p.layout()
	f.screen.updateTitleBars()
	return true
}

// moveEdges moves the floating frame f: its edge on the side dir by delta
// pixels, outward or inward, or all of it towards dir when whole, kept on
// the screen and above the bar.
func (f *Frame) moveEdges(dir direction, delta int, whole bool) {
	sg := f.screen.Geometry()
	maxX, maxY := int(sg.W), int(sg.H)-infoBarOuterH()
	// the outer edges, borders included
	x0, y0 := int(f.g.X), int(f.g.Y)
	x1, y1 := x0+int(f.g.W)+2, y0+int(f.g.H)+2
	switch {
	case whole && (dir == dirLeft || dir == dirRight):
		d := delta
		if dir == dirLeft {
			d = -delta
		}
		d = max(-x0, min(d, maxX-x1))
		x0, x1 = x0+d, x1+d
	case whole:
		d := delta
		if dir == dirUp {
			d = -delta
		}
		d = max(-y0, min(d, maxY-y1))
		y0, y1 = y0+d, y1+d
	case dir == dirLeft:
		x0 = max(0, min(x0-delta, x1-2-minFrameSize()))
	case dir == dirRight:
		x1 = min(maxX, max(x1+delta, x0+2+minFrameSize()))
	case dir == dirUp:
		y0 = max(0, min(y0-delta, y1-2-minFrameSize()))
	case dir == dirDown:
		y1 = min(maxY, max(y1+delta, y0+2+minFrameSize()))
	}
	f.g = Geometry{X: int16(x0), Y: int16(y0), W: uint16(x1 - x0 - 2), H: uint16(y1 - y0 - 2)}
	f.layout()
	f.updateTitleBar()
}

// layoutSnapshot returns what undoes resizing in f's workspace, or f
// itself when floating.
func (f *Frame) layoutSnapshot() func() {
	if f.floating() {
		g := f.g
		return func() {
			f.g = g
			f.layout()
			f.updateTitleBar()
		}
	}
	ratios := map[*Frame]float64{}
	walk(f.workspace.Root, func(n *Frame) { ratios[n] = n.ratio })
	return func() {
		for n, r := range ratios {
			n.ratio = r
		}
		f.workspace.Root.layout()
		f.screen.updateTitleBars()
	}
}

// startResize enters the mode resizing the active frame.
func (wm *Manager) startResize(grow bool) error {
	f := wm.GetActiveFrame()
	undo := f.layoutSnapshot()
	text := func() string {
		if grow {
			return "Resize: arrows grow the frame, - shrinks it, Return confirms, Escape cancels"
		}
		return "Resize: arrows shrink the frame, + grows it, Return confirms, Escape cancels"
	}
	p, err := wm.showPrompt(text(), colorAccent)
	if err != nil {
		return err
	}
	wm.mode = &keyMode{key: func(sym xproto.Keysym) bool {
		if dir, ok := arrowDirection(sym); ok {
			step := resizeStep()
			if !grow {
				step = -step
			}
			f.resize(dir, step)
			return false
		}
		switch sym {
		case XK_Plus, XK_equal, XK_KP_Add:
			grow = true
			p.setText(text())
		case XK_Minus, XK_KP_Subtract:
			grow = false
			p.setText(text())
		case XK_Return, XK_KP_Enter:
			if f.floating() {
				saveScratchpad(f.g)
			}
			return true
		case XK_Escape:
			undo()
			return true
		}
		return false
	}}
	return nil
}

// startMove enters the mode moving the scratchpad, when it is shown.
func (wm *Manager) startMove() error {
	f := wm.GetActiveFrame()
	if !f.floating() {
		return fmt.Errorf("only the scratchpad moves")
	}
	undo := f.layoutSnapshot()
	if _, err := wm.showPrompt("Move: arrows move the scratchpad, Return confirms, Escape cancels", colorAccent); err != nil {
		return err
	}
	wm.mode = &keyMode{key: func(sym xproto.Keysym) bool {
		if dir, ok := arrowDirection(sym); ok {
			f.moveEdges(dir, resizeStep(), true)
			return false
		}
		switch sym {
		case XK_Return, XK_KP_Enter:
			saveScratchpad(f.g)
			return true
		case XK_Escape:
			undo()
			return true
		}
		return false
	}}
	return nil
}

// modeKey hands a key to the mode, and leaves it when done. Modifier keys
// alone do nothing.
func (wm *Manager) modeKey(ev xproto.KeyPressEvent) {
	sym := wm.KeyboardManager.eventKeysym(ev.Detail, ev.State)
	if isModifierKey(sym) {
		return
	}
	if wm.mode.key(sym) {
		wm.mode = nil
		wm.hidePrompt()
	}
}

// stateDir is where fion keeps what outlives it.
func stateDir() string {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "fion")
}

func scratchpadPath() string {
	return filepath.Join(stateDir(), "scratchpad")
}

// saveScratchpad keeps the scratchpad's place and size.
func saveScratchpad(g Geometry) {
	dir := stateDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("scratchpad: %v", err)
		return
	}
	data := fmt.Sprintf("%d %d %d %d\n", g.X, g.Y, g.W, g.H)
	if err := os.WriteFile(scratchpadPath(), []byte(data), 0o600); err != nil {
		log.Printf("scratchpad: %v", err)
	}
}

// savedScratchpad returns the scratchpad's kept place and size, when they
// fit on a screen w×h, above its bar.
func savedScratchpad(w, h int) (Geometry, bool) {
	data, err := os.ReadFile(scratchpadPath())
	if err != nil {
		return Geometry{}, false
	}
	var x, y, gw, gh int
	if n, _ := fmt.Sscanf(string(data), "%d %d %d %d", &x, &y, &gw, &gh); n != 4 {
		return Geometry{}, false
	}
	if x < 0 || y < 0 || gw < minFrameSize() || gh < minFrameSize() ||
		x+gw+2 > w || y+gh+2 > h-infoBarOuterH() {
		return Geometry{}, false
	}
	return Geometry{X: int16(x), Y: int16(y), W: uint16(gw), H: uint16(gh)}, true
}
