package wm

import (
	"encoding/binary"
	"fmt"
	"log"
	"slices"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// A Screen is a monitor: a part of an X screen's root, with its own
// workspaces, bar, panel and scratchpad.
type Screen struct {
	wm *Manager

	atoms      atoms
	screenInfo xproto.ScreenInfo
	monitor    monitor
	Workspaces []*Workspace

	activeWorkspaceIdx int

	scratchpad      *Frame // created the first time it is shown
	scratchpadShown bool

	// the logo, by size and colors, and the operating system's icon
	logos       map[logoKey]logoImage
	osIconImage logoImage
	logoGC      xproto.Gcontext

	// the expanded info bar, created the first time it is shown
	panel *sysPanel

	// the window asking for confirmation, created the first time
	prompt *confirmPrompt

	// the tab shown full screen, if any
	fullscreen fullscreen
}

type atoms struct {
	WM_PROTOCOLS, WM_DELETE_WINDOW, WM_TAKE_FOCUS, WM_STATE                    xproto.Atom
	NET_SUPPORTING_WM_CHECK, NET_SUPPORTED, NET_CLIENT_LIST, NET_ACTIVE_WINDOW xproto.Atom
	NET_WM_NAME, UTF8_STRING, NET_WM_WINDOW_TYPE, NET_WM_WINDOW_TYPE_DOCK      xproto.Atom
}

func (s *Screen) internAtom(name string) xproto.Atom {
	rep, err := xproto.InternAtom(s.Conn(), false, uint16(len(name)), name).Reply()
	if err != nil {
		log.Fatalf("InternAtom %s: %v", name, err)
	}
	return rep.Atom
}

func (s *Screen) getAtoms() atoms {
	return atoms{
		WM_PROTOCOLS:            s.internAtom("WM_PROTOCOLS"),
		WM_DELETE_WINDOW:        s.internAtom("WM_DELETE_WINDOW"),
		WM_TAKE_FOCUS:           s.internAtom("WM_TAKE_FOCUS"),
		WM_STATE:                s.internAtom("WM_STATE"),
		NET_SUPPORTING_WM_CHECK: s.internAtom("_NET_SUPPORTING_WM_CHECK"),
		NET_SUPPORTED:           s.internAtom("_NET_SUPPORTED"),
		NET_CLIENT_LIST:         s.internAtom("_NET_CLIENT_LIST"),
		NET_ACTIVE_WINDOW:       s.internAtom("_NET_ACTIVE_WINDOW"),
		NET_WM_NAME:             s.internAtom("_NET_WM_NAME"),
		UTF8_STRING:             s.internAtom("UTF8_STRING"),
		NET_WM_WINDOW_TYPE:      s.internAtom("_NET_WM_WINDOW_TYPE"),
		NET_WM_WINDOW_TYPE_DOCK: s.internAtom("_NET_WM_WINDOW_TYPE_DOCK"),
	}
}

func newScreen(wm *Manager, screenInfo xproto.ScreenInfo, m monitor, a atoms) (*Screen, error) {
	screen := &Screen{
		wm:         wm,
		screenInfo: screenInfo,
		monitor:    m,
		atoms:      a,
	}
	if ws0, err := newWorkspace(screen); err != nil {
		return nil, err
	} else {
		screen.Workspaces = []*Workspace{ws0}
		ws0.Map()
	}
	return screen, nil
}

// initRoot takes over an X screen's root, and returns the windows it had,
// to adopt, and the atoms.
func (wm *Manager) initRoot(screenInfo xproto.ScreenInfo) ([]xproto.Window, atoms, error) {
	s := &Screen{wm: wm, screenInfo: screenInfo}
	s.atoms = s.getAtoms()

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
		return nil, s.atoms, fmt.Errorf("another WM running: %w", err)
	}

	// before creating any window of our own
	var existing []xproto.Window
	if tree, err := xproto.QueryTree(wm.Conn(), screenInfo.Root).Reply(); err == nil {
		existing = tree.Children
	}

	xproto.ChangeWindowAttributes(wm.Conn(), screenInfo.Root, xproto.CwBackPixel, []uint32{colorBackground})
	xproto.ClearArea(wm.Conn(), false, screenInfo.Root, 0, 0, screenInfo.WidthInPixels, screenInfo.HeightInPixels)

	if err := s.initEWMH(); err != nil {
		return nil, s.atoms, err
	}
	return existing, s.atoms, nil
}

func (s *Screen) Conn() *xgb.Conn {
	return s.wm.Conn()
}

// Geometry is the monitor's, in the root's coordinates.
func (s *Screen) Geometry() Geometry {
	return s.monitor.g
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

// updateTitleBars redraws the title bars of every frame on the screen.
func (s *Screen) updateTitleBars() {
	for _, ws := range s.Workspaces {
		ws.updateTitleBars()
	}
	if s.scratchpad != nil {
		s.scratchpad.updateTitleBar()
	}
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
	// the one shown, whose place moved with the removal
	s.activeWorkspaceIdx = slices.Index(s.Workspaces, new)

	new.Map()
	old.Destroy()
}

// workAreaH is the height the workspaces' frames take: the screen's but
// for the info bar at the bottom, and the panel above it when shown.
func (s *Screen) workAreaH() int {
	h := int(s.Geometry().H) - infoBarOuterH()
	if s.panel != nil && s.panel.shown {
		h -= s.panel.h
	}
	return h
}

// layoutWorkspaces fits the workspaces' frames to the work area.
func (s *Screen) layoutWorkspaces() {
	w, h := s.Geometry().W, uint16(s.workAreaH())
	for _, ws := range s.Workspaces {
		if ws.Root == nil || (ws.Root.g.W == w && ws.Root.g.H == h) {
			continue
		}
		ws.Root.g.W, ws.Root.g.H = w, h
		ws.Root.layout()
		// the logo, centered again
		xproto.ClearArea(s.Conn(), true, ws.Root.window, 0, 0, 0, 0)
	}
}
