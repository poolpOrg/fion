package wm

import (
	"fmt"

	"github.com/jezek/xgb/xproto"
)

// Mod+? shows a cheat sheet of the bindings, made from the bindings
// themselves, centered on the screen. Any key or click closes it.

type cheatSheet struct {
	window xproto.Window
	gc     xproto.Gcontext
	lines  []string
	shown  bool
}

// cheatSheetLines lists the bindings, and what the mouse does.
func cheatSheetLines(mod string) []string {
	rows := [][2]string{}
	for _, b := range bindings {
		rows = append(rows, [2]string{b.keys(mod), b.desc})
	}
	rows = append(rows,
		[2]string{mod + "+?", "this cheat sheet"},
		[2]string{mod + "+Escape", "quit fion"},
		[2]string{"", ""},
		[2]string{"click a tab", "select it"},
		[2]string{"drag a tab", "move it to another frame, or within its bar"},
		[2]string{"click the bar", "show / hide the system panel"},
	)
	w := 0
	for _, r := range rows {
		w = max(w, len(r[0]))
	}
	lines := []string{"fion keys", ""}
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("%-*s  %s", w, r[0], r[1]))
	}
	return append(lines, "", "any key or click closes this")
}

func (wm *Manager) cheatSheetShown() bool {
	return wm.cheat != nil && wm.cheat.shown
}

func (wm *Manager) showCheatSheet() error {
	s := wm.GetActiveScreen()
	conn := wm.Conn()
	if wm.cheat == nil {
		w, err := xproto.NewWindowId(conn)
		if err != nil {
			return err
		}
		xproto.CreateWindow(conn, s.Info().RootDepth, w, s.Info().Root, 0, 0, 1, 1, 1,
			xproto.WindowClassInputOutput, s.Info().RootVisual,
			xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask,
			[]uint32{colorBar, colorAccent, xproto.EventMaskExposure | xproto.EventMaskButtonPress})
		gc, err := xproto.NewGcontextId(conn)
		if err != nil {
			return err
		}
		xproto.CreateGC(conn, gc, xproto.Drawable(w), xproto.GcForeground|xproto.GcBackground,
			[]uint32{colorText, colorBar})
		if fid, ok := openFont(conn, font.plain); ok {
			xproto.ChangeGC(conn, gc, xproto.GcFont, []uint32{uint32(fid)})
			xproto.CloseFont(conn, fid)
		}
		wm.cheat = &cheatSheet{window: w, gc: gc}
	}
	c := wm.cheat
	if err := wm.KeyboardManager.GrabKeyboard(s.Info().Root); err != nil {
		return err
	}
	c.lines = cheatSheetLines(wm.KeyboardManager.ModName)
	c.shown = true

	cols := 0
	for _, l := range c.lines {
		cols = max(cols, len(l))
	}
	g := s.Geometry()
	w := min((cols+4)*charW(), int(g.W)-2)
	h := min((len(c.lines)+2)*panelLineH(), int(g.H)-2)
	xproto.ConfigureWindow(conn, c.window,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight|xproto.ConfigWindowStackMode,
		[]uint32{uint32((int(g.W) - w - 2) / 2), uint32((int(g.H) - h - 2) / 2), uint32(w), uint32(h), xproto.StackModeAbove})
	xproto.MapWindow(conn, c.window)
	wm.drawCheatSheet()
	return nil
}

func (wm *Manager) drawCheatSheet() {
	c := wm.cheat
	conn := wm.Conn()
	xproto.ClearArea(conn, false, c.window, 0, 0, 0, 0)
	for i, l := range c.lines {
		fg := uint32(colorText)
		switch {
		case i == 0:
			fg = colorAccent
		case i == len(c.lines)-1:
			fg = colorDim
		}
		if len(l) > 255 {
			l = l[:255]
		}
		xproto.ChangeGC(conn, c.gc, xproto.GcForeground, []uint32{fg})
		xproto.ImageText8(conn, byte(len(l)), xproto.Drawable(c.window), c.gc,
			int16(2*charW()), int16((i+1)*panelLineH()+font.ascent), l)
	}
}

func (wm *Manager) hideCheatSheet() {
	wm.cheat.shown = false
	xproto.UnmapWindow(wm.Conn(), wm.cheat.window)
	wm.KeyboardManager.UngrabKeyboard()
}

// cheatSheetKey closes the cheat sheet on any key but a modifier.
func (wm *Manager) cheatSheetKey(ev xproto.KeyPressEvent) {
	if !isModifierKey(wm.KeyboardManager.eventKeysym(ev.Detail, ev.State)) {
		wm.hideCheatSheet()
	}
}
