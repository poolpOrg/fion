package wm

import (
	"fmt"
	"log"

	"github.com/jezek/xgb/xproto"
)

// Mod+d closes what has the focus: the active window, or the frame when it
// is empty, or the workspace when it is a single empty frame, but for the
// last one. It asks first, in a line at the top of the screen: d again
// confirms, any other key cancels.

// confirmPrompt is the line at the top of the screen that asks, or says
// what keys do in a mode.
type confirmPrompt struct {
	screen *Screen
	window xproto.Window
	gc     xproto.Gcontext
	text   string
	shown  bool
	action func() // what confirming Mod+d's question does

	noticeShown bool // showing a notice, which doesn't take the keyboard
}

// closeQuestion is what Mod+d asks about the active frame, "" when there is
// nothing to close.
func (wm *Manager) closeQuestion() string {
	if d := wm.focusedDialog(); d != 0 {
		if name := getWindowName(wm.Conn(), d); name != "" {
			return fmt.Sprintf("Close the dialog %q?", name)
		}
		return "Close this dialog?"
	}
	frame := wm.GetActiveFrame()
	if win := frame.GetActiveClient(); win != 0 {
		name := getWindowName(wm.Conn(), win)
		if name == "" {
			name = "this window"
		} else {
			name = fmt.Sprintf("%q", name)
		}
		if c := wm.Clients[win]; c != nil && c.closeRequested {
			return fmt.Sprintf("Kill %s, which didn't close when asked?", name)
		}
		return fmt.Sprintf("Close %s?", name)
	}
	switch {
	case frame.floating():
		return ""
	case frame.parent != nil:
		return "Remove this empty frame?"
	}
	s := wm.GetActiveScreen()
	if len(s.Workspaces) <= 1 {
		return ""
	}
	_, n, _ := frame.workspace.position()
	return fmt.Sprintf("Remove workspace %d?", n)
}

// requestClose asks whether to close what has the focus.
func (wm *Manager) requestClose() error {
	question := wm.closeQuestion()
	if question == "" {
		return nil
	}
	p, err := wm.showPrompt(question+"  d: yes, any other key: no", colorAlert)
	if err != nil {
		return err
	}
	dialog := wm.focusedDialog()
	p.action = func() {
		if dialog != 0 {
			wm.closeDialog(dialog)
			return
		}
		if err := wm.closeActive(); err != nil {
			log.Printf("close: %v", err)
		}
	}
	wm.confirm = p
	return nil
}

// showPrompt shows a line of text at the top of the screen, bordered in
// color, and takes the keyboard until hidePrompt.
func (wm *Manager) showPrompt(text string, color uint32) (*confirmPrompt, error) {
	s := wm.GetActiveScreen()
	if err := wm.KeyboardManager.GrabKeyboard(s.Info().Root); err != nil {
		return nil, err
	}
	p, err := wm.promptWindow(s)
	if err != nil {
		wm.KeyboardManager.UngrabKeyboard()
		return nil, err
	}
	p.text, p.shown = text, true
	xproto.ChangeWindowAttributes(wm.Conn(), p.window, xproto.CwBorderPixel, []uint32{color})
	p.place()
	xproto.MapWindow(wm.Conn(), p.window)
	p.draw()
	return p, nil
}

// setText changes the prompt's text, as when a mode changes.
func (p *confirmPrompt) setText(text string) {
	p.text = text
	p.place()
	p.draw()
}

// place sizes the prompt to its text, centered, in the upper part of the
// screen.
func (p *confirmPrompt) place() {
	g := p.screen.Geometry()
	w := min((len(p.text)+2)*charW(), int(g.W)-2)
	xproto.ConfigureWindow(p.screen.Conn(), p.window,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowStackMode,
		[]uint32{uint32(int(g.X) + (int(g.W)-w-2)/2), uint32(int(g.Y) + int(g.H)/4), uint32(w), xproto.StackModeAbove})
}

// hidePrompt hides the prompt and gives the keyboard back.
func (wm *Manager) hidePrompt() {
	if p := wm.GetActiveScreen().prompt; p != nil && p.shown {
		p.shown = false
		xproto.UnmapWindow(wm.Conn(), p.window)
	}
	wm.KeyboardManager.UngrabKeyboard()
}

// promptWindow returns the screen's prompt window, made the first time.
func (wm *Manager) promptWindow(s *Screen) (*confirmPrompt, error) {
	if s.prompt != nil {
		return s.prompt, nil
	}
	conn := wm.Conn()
	w, err := xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}
	xproto.CreateWindow(conn, s.Info().RootDepth, w, s.Info().Root,
		0, 0, 1, uint16(launcherInputH()), 1,
		xproto.WindowClassInputOutput, s.Info().RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask,
		[]uint32{colorTab, colorAlert, xproto.EventMaskExposure})
	gc, err := xproto.NewGcontextId(conn)
	if err != nil {
		return nil, err
	}
	xproto.CreateGC(conn, gc, xproto.Drawable(w), xproto.GcForeground|xproto.GcBackground,
		[]uint32{colorText, colorTab})
	if fid, ok := openFont(conn, font.plain); ok {
		xproto.ChangeGC(conn, gc, xproto.GcFont, []uint32{uint32(fid)})
		xproto.CloseFont(conn, fid)
	}
	s.prompt = &confirmPrompt{screen: s, window: w, gc: gc}
	return s.prompt, nil
}

func (p *confirmPrompt) draw() {
	text := p.text
	if len(text) > 255 {
		text = text[:255]
	}
	conn := p.screen.Conn()
	xproto.ClearArea(conn, false, p.window, 0, 0, 0, 0)
	xproto.ImageText8(conn, byte(len(text)), xproto.Drawable(p.window), p.gc,
		int16(charW()), int16(baseline(launcherInputH())), text)
}

// confirmKey answers the prompt: d, with or without modifiers, confirms,
// any other key cancels. Modifier keys alone don't answer.
func (wm *Manager) confirmKey(ev xproto.KeyPressEvent) {
	sym := wm.KeyboardManager.eventKeysym(ev.Detail, ev.State)
	if isModifierKey(sym) {
		return
	}
	p := wm.confirm
	wm.confirm = nil
	wm.hidePrompt()
	if sym == XK_d || sym == XK_D {
		p.action()
	}
}
