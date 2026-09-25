package wm

import (
	"log"
	"os"

	"github.com/jezek/xgb/xproto"
)

// The launcher's window: an input line and the best matches under it,
// centered in the upper part of the screen. It takes the keyboard while it
// is open.

const (
	XK_Return    xproto.Keysym = 0xFF0D
	XK_KP_Enter  xproto.Keysym = 0xFF8D
	XK_BackSpace xproto.Keysym = 0xFF08
	XK_Tab       xproto.Keysym = 0xFF09

	launcherResults = 10
)

// the launcher's sizes, following the font
func launcherW() int      { return scaled(640) }
func launcherInputH() int { return textH() + 11 }
func launcherLineH() int  { return textH() + 5 }

type launcherUI struct {
	*launcher
	screen *Screen
	window xproto.Window
	gc     xproto.Gcontext
	shown  bool
}

func (wm *Manager) launcherOpen() bool {
	return wm.launcher != nil && wm.launcher.shown
}

// openLauncher shows the launcher, with its sources read afresh.
func (wm *Manager) openLauncher() error {
	s := wm.GetActiveScreen()
	if wm.launcher == nil || wm.launcher.screen != s {
		ui, err := newLauncherUI(s)
		if err != nil {
			return err
		}
		wm.launcher = ui
	}
	ui := wm.launcher

	// the keyboard first: what is typed while the sources are read goes to
	// the launcher, once it is drawn
	if err := wm.KeyboardManager.GrabKeyboard(s.Info().Root); err != nil {
		return err
	}
	items := launchItems(desktopApps(applicationDirs()), pathCommands(os.Getenv("PATH")),
		loadHistory(historyPath()))
	ui.launcher = newLauncher(items)
	ui.shown = true
	xproto.MapWindow(wm.Conn(), ui.window)
	xproto.ConfigureWindow(wm.Conn(), ui.window, xproto.ConfigWindowStackMode,
		[]uint32{xproto.StackModeAbove})
	wm.drawLauncher()
	return nil
}

func newLauncherUI(s *Screen) (*launcherUI, error) {
	conn := s.Conn()
	w, err := xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}
	g := s.Geometry()
	width := min(uint16(launcherW()), g.W-40)
	xproto.CreateWindow(conn, s.Info().RootDepth, w, s.Info().Root,
		int16((g.W-width-2)/2), int16(g.H/4), width, uint16(launcherInputH()), 1,
		xproto.WindowClassInputOutput, s.Info().RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask,
		[]uint32{colorBar, colorAccent, xproto.EventMaskExposure})

	gc, err := xproto.NewGcontextId(conn)
	if err != nil {
		return nil, err
	}
	xproto.CreateGC(conn, gc, xproto.Drawable(w), xproto.GcForeground|xproto.GcBackground,
		[]uint32{colorText, colorBar})
	if fid, ok := openFont(conn, font.plain); ok {
		xproto.ChangeGC(conn, gc, xproto.GcFont, []uint32{uint32(fid)})
		xproto.CloseFont(conn, fid)
	}

	return &launcherUI{screen: s, window: w, gc: gc}, nil
}

func (wm *Manager) closeLauncher() {
	ui := wm.launcher
	ui.shown = false
	xproto.UnmapWindow(wm.Conn(), ui.window)
	wm.KeyboardManager.UngrabKeyboard()
}

// drawLauncher sizes the window to the matches shown and draws it.
func (wm *Manager) drawLauncher() {
	ui := wm.launcher
	conn, d := wm.Conn(), xproto.Drawable(ui.window)
	width := min(launcherW(), int(ui.screen.Geometry().W)-40)
	chars := (width - 12) / charW()

	n := min(len(ui.matches), launcherResults)
	height := launcherInputH()
	if n > 0 {
		height += n*launcherLineH() + 4
	}
	xproto.ConfigureWindow(conn, ui.window, xproto.ConfigWindowHeight, []uint32{uint32(height)})

	fill := func(x, y, w, h int, color uint32) {
		xproto.ChangeGC(conn, ui.gc, xproto.GcForeground, []uint32{color})
		xproto.PolyFillRectangle(conn, d, ui.gc,
			[]xproto.Rectangle{{X: int16(x), Y: int16(y), Width: uint16(w), Height: uint16(h)}})
	}
	text := func(x, y int, s string, fg, bg uint32) {
		if len(s) > 255 {
			s = s[:255]
		}
		xproto.ChangeGC(conn, ui.gc, xproto.GcForeground|xproto.GcBackground, []uint32{fg, bg})
		xproto.ImageText8(conn, byte(len(s)), d, ui.gc, int16(x), int16(y), s)
	}

	// the input line, showing its end when too long
	fill(0, 0, width, launcherInputH(), colorTab)
	input := "> " + ui.query + "_"
	if len(input) > chars {
		input = input[len(input)-chars:]
	}
	text(6, baseline(launcherInputH()), input, colorText, colorTab)

	fill(0, launcherInputH(), width, height-launcherInputH(), colorBar)
	for i := range n {
		it := ui.items[ui.matches[i]]
		y := launcherInputH() + 2 + i*launcherLineH()
		bg, fg, dim := uint32(colorBar), uint32(colorText), uint32(colorDim)
		if i == ui.selected {
			bg, fg, dim = colorAccent, colorAccentText, colorAccentText
			fill(0, y, width, launcherLineH(), bg)
		}

		// the label, then what runs when it differs, then where it comes from
		tag := map[launchKind]string{kindApp: "app", kindHistory: "history"}[it.kind]
		room := chars - len(tag) - 2
		label := it.label
		if len(label) > room {
			label = label[:max(room, 0)]
		}
		text(6, y+baseline(launcherLineH()), label, fg, bg)
		if it.command != it.label && room-len(label) > 4 {
			command := it.command
			if len(command) > room-len(label)-2 {
				command = command[:room-len(label)-2]
			}
			text(6+(len(label)+2)*charW(), y+baseline(launcherLineH()), command, dim, bg)
		}
		if tag != "" {
			text(width-6-len(tag)*charW(), y+baseline(launcherLineH()), tag, dim, bg)
		}
	}
}

// launcherKey handles a key pressed while the launcher is open.
func (wm *Manager) launcherKey(ev xproto.KeyPressEvent) {
	km := wm.KeyboardManager
	mods := ev.State &^ (xproto.ModMaskLock | km.Num)
	sym := km.eventKeysym(ev.Detail, ev.State)
	ctrl := mods&xproto.ModMaskControl != 0
	l := wm.launcher.launcher

	switch {
	case mods == km.Mod && sym == XK_Return, sym == XK_Escape:
		wm.closeLauncher()
		return
	case sym == XK_Return, sym == XK_KP_Enter:
		line := l.line()
		wm.closeLauncher()
		if line != "" {
			if l.needsTerminal(line, launcherOpensWindows) {
				args := inTerminal(line)
				wm.spawn(args[0], args[1:]...)
			} else {
				wm.spawn("/bin/sh", "-c", line)
			}
			if err := appendHistory(historyPath(), line); err != nil {
				log.Printf("launcher history: %v", err)
			}
		}
		return
	case sym == XK_Up, ctrl && sym == XK_p:
		l.move(-1)
	case sym == XK_Down, ctrl && sym == XK_n:
		l.move(1)
	case sym == XK_Tab:
		l.complete()
	case sym == XK_BackSpace:
		l.backspace()
	case ctrl && sym == XK_u:
		l.setQuery("")
	case ctrl && sym == XK_w:
		l.deleteWord()
	case !ctrl && sym >= 0x20 && sym <= 0x7e:
		l.setQuery(l.query + string(rune(sym)))
	default:
		return
	}
	wm.drawLauncher()
}
