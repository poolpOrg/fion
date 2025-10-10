package fion

import (
	"fmt"

	"github.com/BurntSushi/xgb/xproto"
)

// Screen holds multiple workspaces and active WS index.
type Screen struct {
	wm         *WM
	ScreenInfo xproto.ScreenInfo
	Workspaces []*Workspace

	activeWorkspaceIdx int
}

func newScreen(wm *WM, scr xproto.ScreenInfo) (*Screen, error) {
	screen := &Screen{
		wm:         wm,
		ScreenInfo: scr,
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
	if err := xproto.ChangeWindowAttributesChecked(wm.Conn(), scr.Root, xproto.CwEventMask, []uint32{mask}).Check(); err != nil {
		return nil, fmt.Errorf("another WM running: %w", err)
	}

	xproto.ChangeWindowAttributes(wm.Conn(), scr.Root, xproto.CwBackPixel, []uint32{scr.BlackPixel})
	xproto.ClearArea(wm.Conn(), false, scr.Root, 0, 0, scr.WidthInPixels, scr.HeightInPixels)

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

func (sc *Screen) Geometry() Rect {
	scr := sc.ScreenInfo
	return Rect{0, 0, uint16(scr.WidthInPixels), uint16(scr.HeightInPixels)}
}

func (sc *Screen) GetActiveWorkspace() *Workspace {
	if sc.activeWorkspaceIdx < 0 || sc.activeWorkspaceIdx >= len(sc.Workspaces) {
		return nil
	}
	return sc.Workspaces[sc.activeWorkspaceIdx]
}

func (sc *Screen) newWorkspace() (*Workspace, error) {
	ws, err := newWorkspace(sc)
	if err != nil {
		return nil, err
	}
	sc.Workspaces = append(sc.Workspaces, ws)
	sc.activeWorkspaceIdx = len(sc.Workspaces) - 1
	return ws, nil
}

func (sc *Screen) cycleWorkspaceLeft() *Workspace {
	sc.activeWorkspaceIdx = (sc.activeWorkspaceIdx + len(sc.Workspaces) - 1) % len(sc.Workspaces)
	return sc.Workspaces[sc.activeWorkspaceIdx]
}

func (sc *Screen) cycleWorkspaceRight() *Workspace {
	sc.activeWorkspaceIdx = (sc.activeWorkspaceIdx + 1) % len(sc.Workspaces)
	return sc.Workspaces[sc.activeWorkspaceIdx]
}

func (sc *Screen) removeWorkspace() {
	if len(sc.Workspaces) <= 1 {
		return
	}

	old := sc.GetActiveWorkspace()
	new := sc.cycleWorkspaceRight()

	workspaces := []*Workspace{}
	for _, w := range sc.Workspaces {
		if w != old {
			workspaces = append(workspaces, w)
		}
	}

	sc.Workspaces = workspaces

	if sc.activeWorkspaceIdx >= len(sc.Workspaces) {
		sc.activeWorkspaceIdx = 0
	}

	new.Map()
	old.Destroy()
}

func (s *Screen) initEWMH() error {
	wm := s.wm
	X, A := wm.Conn(), wm.Atoms
	w, err := xproto.NewWindowId(X)
	if err != nil {
		return err
	}
	xproto.CreateWindow(wm.Conn(), s.ScreenInfo.RootDepth, w, s.ScreenInfo.Root, 0, 0, 1, 1, 0,
		xproto.WindowClassInputOutput, s.ScreenInfo.RootVisual, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange})
	//wm.SupportingWin = w
	wm.setProp32(s.ScreenInfo.Root, A.NET_SUPPORTING_WM_CHECK, xproto.AtomWindow, uint32(w))
	wm.setProp32(w, A.NET_SUPPORTING_WM_CHECK, xproto.AtomWindow, uint32(w))
	wm.setPropStr(w, A.NET_WM_NAME, A.UTF8_STRING, "fion")
	supported := []xproto.Atom{A.NET_SUPPORTED, A.NET_SUPPORTING_WM_CHECK, A.NET_CLIENT_LIST, A.NET_ACTIVE_WINDOW}
	wm.setPropAtoms(s.ScreenInfo.Root, A.NET_SUPPORTED, supported)
	//wm.updateClientList()
	return nil
}
