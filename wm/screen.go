package wm

import (
	"encoding/binary"
	"fmt"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

// Screen holds multiple workspaces and active WS index.
type Screen struct {
	wm *Manager

	atoms      Atoms
	screenInfo xproto.ScreenInfo
	Workspaces []*Workspace

	activeWorkspaceIdx int
}

func newScreen(wm *Manager, screenInfo xproto.ScreenInfo) (*Screen, error) {
	screen := &Screen{
		wm:         wm,
		atoms:      getAtoms(wm.Conn()),
		screenInfo: screenInfo,
	}

	mask := uint32(
		xproto.EventMaskSubstructureRedirect |
			xproto.EventMaskSubstructureNotify |
			xproto.EventMaskPropertyChange |
			xproto.EventMaskButtonPress |
			xproto.EventMaskButtonRelease |
			xproto.EventMaskPointerMotion |
			xproto.EventMaskKeyPress,
	)
	if err := xproto.ChangeWindowAttributesChecked(wm.Conn(), screenInfo.Root, xproto.CwEventMask, []uint32{mask}).Check(); err != nil {
		return nil, fmt.Errorf("another WM running: %w", err)
	}

	xproto.ChangeWindowAttributes(wm.Conn(), screenInfo.Root, xproto.CwBackPixel, []uint32{screenInfo.BlackPixel})
	xproto.ClearArea(wm.Conn(), false, screenInfo.Root, 0, 0, screenInfo.WidthInPixels, screenInfo.HeightInPixels)

	if err := screen.initEWMH(); err != nil {
		return nil, err
	}

	if ws0, err := newWorkspace(screen); err != nil {
		return nil, err
	} else {
		screen.Workspaces = []*Workspace{ws0}
		ws0.Map()
	}
	return screen, nil
}

func (s *Screen) Conn() *xgb.Conn {
	return s.wm.Conn()
}

func (s *Screen) Geometry() Rect {
	return Rect{0, 0, uint16(s.Info().WidthInPixels), uint16(s.Info().HeightInPixels)}
}

func (s *Screen) Info() xproto.ScreenInfo {
	return s.screenInfo
}

func (s *Screen) initEWMH() error {
	wm := s.wm
	w, err := xproto.NewWindowId(s.Conn())
	if err != nil {
		return err
	}
	xproto.CreateWindow(wm.Conn(), s.Info().RootDepth, w, s.Info().Root, 0, 0, 1, 1, 0,
		xproto.WindowClassInputOutput, s.Info().RootVisual, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange})
	//wm.SupportingWin = w
	s.setProp32(s.Info().Root, s.atoms.NET_SUPPORTING_WM_CHECK, xproto.AtomWindow, uint32(w))
	s.setProp32(w, s.atoms.NET_SUPPORTING_WM_CHECK, xproto.AtomWindow, uint32(w))
	s.setPropStr(w, s.atoms.NET_WM_NAME, s.atoms.UTF8_STRING, "fion")
	supported := []xproto.Atom{s.atoms.NET_SUPPORTED, s.atoms.NET_SUPPORTING_WM_CHECK, s.atoms.NET_CLIENT_LIST, s.atoms.NET_ACTIVE_WINDOW}
	s.setPropAtoms(s.Info().Root, s.atoms.NET_SUPPORTED, supported)
	//wm.updateClientList()
	return nil
}

func (s *Screen) setProp32(win xproto.Window, prop, typ xproto.Atom, v uint32) {
	xproto.ChangeProperty(s.Conn(), xproto.PropModeReplace, win, prop, typ, 32, 1, []uint8{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
}

func (s *Screen) setPropStr(win xproto.Window, prop, typ xproto.Atom, str string) {
	data := []byte(str)
	xproto.ChangeProperty(s.Conn(), xproto.PropModeReplace, win, prop, typ, 8, uint32(len(data)), data)
}

func (s *Screen) setPropAtoms(win xproto.Window, prop xproto.Atom, atoms []xproto.Atom) {
	buf := make([]byte, 4*len(atoms))
	for i, a := range atoms {
		binary.LittleEndian.PutUint32(buf[i*4:(i+1)*4], uint32(a))
	}
	xproto.ChangeProperty(s.Conn(), xproto.PropModeReplace, win, prop, xproto.AtomAtom, 32, uint32(len(atoms)), buf)
}

func (s *Screen) GetActiveWorkspace() *Workspace {
	if s.activeWorkspaceIdx < 0 || s.activeWorkspaceIdx >= len(s.Workspaces) {
		return nil
	}
	return s.Workspaces[s.activeWorkspaceIdx]
}

func (s *Screen) newWorkspace() (*Workspace, error) {
	ws, err := newWorkspace(s)
	if err != nil {
		return nil, err
	}
	s.Workspaces = append(s.Workspaces, ws)
	s.activeWorkspaceIdx = len(s.Workspaces) - 1
	return ws, nil
}

func (s *Screen) cycleWorkspaceLeft() *Workspace {
	s.activeWorkspaceIdx = (s.activeWorkspaceIdx + len(s.Workspaces) - 1) % len(s.Workspaces)
	return s.Workspaces[s.activeWorkspaceIdx]
}

func (s *Screen) cycleWorkspaceRight() *Workspace {
	s.activeWorkspaceIdx = (s.activeWorkspaceIdx + 1) % len(s.Workspaces)
	return s.Workspaces[s.activeWorkspaceIdx]
}

func (s *Screen) removeWorkspace() {
	if len(s.Workspaces) <= 1 {
		return
	}

	old := s.GetActiveWorkspace()
	new := s.cycleWorkspaceRight()

	workspaces := []*Workspace{}
	for _, w := range s.Workspaces {
		if w != old {
			workspaces = append(workspaces, w)
		}
	}

	s.Workspaces = workspaces

	if s.activeWorkspaceIdx >= len(s.Workspaces) {
		s.activeWorkspaceIdx = 0
	}

	new.Map()
	old.Destroy()
}
