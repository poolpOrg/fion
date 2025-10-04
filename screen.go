package main

import (
	"fmt"

	"github.com/BurntSushi/xgb/xproto"
)

// Screen holds multiple workspaces and active WS index.
type Screen struct {
	wm                 *WM
	ScreenInfo         xproto.ScreenInfo
	Workspaces         []*Workspace
	activeWorkspaceIdx int
}

func newScreen(wm *WM, id int, scr xproto.ScreenInfo) (*Screen, error) {
	screen := &Screen{
		wm:         wm,
		ScreenInfo: scr,
	}
	if ws0, err := newWorkspace(screen); err != nil {
		return nil, err
	} else {
		screen.Workspaces = []*Workspace{ws0}
	}
	return screen, nil
}

func (sc *Screen) Geom() Rect {
	scr := sc.ScreenInfo
	return Rect{0, 0, uint16(scr.WidthInPixels), uint16(scr.HeightInPixels)}
}

func (sc *Screen) ActiveWorkspace() *Workspace {
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
	fmt.Println("Switched to workspace", sc.activeWorkspaceIdx)
	return sc.Workspaces[sc.activeWorkspaceIdx]
}

func (sc *Screen) cycleWorkspaceRight() *Workspace {
	sc.activeWorkspaceIdx = (sc.activeWorkspaceIdx + 1) % len(sc.Workspaces)
	fmt.Println("Switched to workspace", sc.activeWorkspaceIdx)
	return sc.Workspaces[sc.activeWorkspaceIdx]
}

func (sc *Screen) removeWorkspace() {
	if len(sc.Workspaces) <= 1 {
		return
	}

	old := sc.ActiveWorkspace()
	new := sc.cycleWorkspaceRight()

	workspaces := []*Workspace{}
	for _, w := range sc.Workspaces {
		if w != old {
			workspaces = append(workspaces, w)
		}
	}

	fmt.Println("Removed workspace", old, "now active is", new, "index", sc.activeWorkspaceIdx)

	sc.Workspaces = workspaces

	if sc.activeWorkspaceIdx >= len(sc.Workspaces) {
		sc.activeWorkspaceIdx = 0
	}

	new.Map()
	old.Destroy()
}
