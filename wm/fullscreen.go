package wm

import (
	"fmt"

	"github.com/jezek/xgb/xproto"
)

// Mod+f shows the active tab full screen, over the bar and the other
// frames, and Mod+f again puts it back in its frame. Any other binding, or
// a new window, puts it back first, so that the frames never act on a tab
// they don't hold.

type fullscreen struct {
	window xproto.Window // over the whole screen, holding the client
	client xproto.Window // 0 when no tab is shown full screen
}

// toggleFullscreen shows the active tab full screen, or puts it back.
func (s *Screen) toggleFullscreen() error {
	if s.fullscreen.client != 0 {
		s.leaveFullscreen()
		return nil
	}
	win := s.wm.GetActiveFrame().GetActiveClient()
	c, ok := s.wm.Clients[win]
	if !ok {
		return fmt.Errorf("no tab to show full screen")
	}

	conn, g := s.Conn(), s.Geometry()
	if s.fullscreen.window == 0 {
		w, err := xproto.NewWindowId(conn)
		if err != nil {
			return err
		}
		xproto.CreateWindow(conn, s.Info().RootDepth, w, s.Info().Root,
			g.X, g.Y, g.W, g.H, 0,
			xproto.WindowClassInputOutput, s.Info().RootVisual,
			xproto.CwBackPixel|xproto.CwOverrideRedirect, []uint32{colorBackground, 1})
		s.fullscreen.window = w
	}
	xproto.MapWindow(conn, s.fullscreen.window)
	// the monitor may have moved since
	xproto.ConfigureWindow(conn, s.fullscreen.window,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight|xproto.ConfigWindowStackMode,
		[]uint32{uint32(g.X), uint32(g.Y), uint32(g.W), uint32(g.H), xproto.StackModeAbove})

	// reparenting a mapped window unmaps it first
	if c.mapped {
		c.ignoreUnmap++
	}
	xproto.ConfigureWindow(conn, win, xproto.ConfigWindowBorderWidth, []uint32{0})
	xproto.ReparentWindow(conn, win, s.fullscreen.window, 0, 0)
	xproto.ConfigureWindow(conn, win,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
		[]uint32{0, 0, uint32(g.W), uint32(g.H)})
	s.fullscreen.client = win
	s.wm.setFullscreenState(win, true)
	s.wm.updateFocus()
	return nil
}

// leaveFullscreen puts the tab shown full screen back in its frame.
func (s *Screen) leaveFullscreen() {
	win := s.fullscreen.client
	if win == 0 {
		return
	}
	s.fullscreen.client = 0
	conn := s.Conn()
	if c, ok := s.wm.Clients[win]; ok {
		s.wm.setFullscreenState(win, false)
		if c.mapped {
			c.ignoreUnmap++
		}
		xproto.ConfigureWindow(conn, win, xproto.ConfigWindowBorderWidth, []uint32{1})
		xproto.ReparentWindow(conn, win, c.frame.window, 0, int16(titleH()))
		xproto.ConfigureWindow(conn, win,
			xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
			c.frame.clientGeometry())
	}
	xproto.UnmapWindow(conn, s.fullscreen.window)
	s.wm.updateFocus()
}

// forgetFullscreen drops the full screen tab when its window goes away.
func (s *Screen) forgetFullscreen(win xproto.Window) {
	if win != 0 && s.fullscreen.client == win {
		s.fullscreen.client = 0
		xproto.UnmapWindow(s.Conn(), s.fullscreen.window)
	}
}
