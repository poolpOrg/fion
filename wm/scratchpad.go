package wm

import "github.com/jezek/xgb/xproto"

// The scratchpad follows ion's mod_sp: a single frame per screen, floating
// centered above the workspaces, that Mod+space shows and hides. It stays
// shown across workspace switches. While shown it is the active frame, so
// new windows open in it and the tab bindings act on it, and it holds the
// input focus.

const scratchpadW, scratchpadH = 640, 480

// scratchpadGeometry centers the scratchpad, 1px border included, on the
// screen.
func (s *Screen) scratchpadGeometry() Geometry {
	g := s.Geometry()
	w, h := min(uint16(scaled(scratchpadW)), g.W-2), min(uint16(scaled(scratchpadH)), g.H-2)
	return Geometry{
		X: int16((g.W - w - 2) / 2),
		Y: int16((g.H - h - 2) / 2),
		W: w,
		H: h,
	}
}

// toggleScratchpad shows the scratchpad, creating it the first time, or
// hides it.
func (s *Screen) toggleScratchpad() error {
	if s.scratchpad == nil {
		f, err := newFrame(s, nil, nil, s.scratchpadGeometry())
		if err != nil {
			return err
		}
		s.scratchpad = f
	}

	s.scratchpadShown = !s.scratchpadShown
	if s.scratchpadShown {
		s.scratchpad.Map()
		s.raiseScratchpad()
	} else {
		s.scratchpad.Unmap()
	}
	s.updateTitleBars()
	s.wm.updateFocus()
	return nil
}

// raiseScratchpad keeps the shown scratchpad above the workspaces, which
// stack in creation order.
func (s *Screen) raiseScratchpad() {
	if !s.scratchpadShown {
		return
	}
	xproto.ConfigureWindow(s.Conn(), s.scratchpad.window, xproto.ConfigWindowStackMode,
		[]uint32{xproto.StackModeAbove})
}

// updateFocus gives the input focus to the scratchpad's active client while
// it is shown, and lets it follow the pointer otherwise.
func (wm *Manager) updateFocus() {
	focus := xproto.Window(xproto.InputFocusPointerRoot)
	if s := wm.GetActiveScreen(); s != nil && s.scratchpadShown {
		if c := s.scratchpad.GetActiveClient(); c != 0 {
			focus = c
		}
	}
	xproto.SetInputFocus(wm.Conn(), xproto.InputFocusPointerRoot, focus, xproto.TimeCurrentTime)
}
