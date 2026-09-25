package wm

import (
	"os"
	"os/exec"
	"sort"

	"github.com/jezek/xgb/xproto"
	"github.com/shirou/gopsutil/v4/process"
)

// Mod+t opens a terminal in the directory of the one with the focus: that
// of the shell the terminal runs, its first child, found through the
// _NET_WM_PID of its window.

// windowPID is the process a window's _NET_WM_PID names, 0 when none.
func (wm *Manager) windowPID(win xproto.Window) int32 {
	a := wm.Screens[0].atoms
	r, err := xproto.GetProperty(wm.Conn(), false, win, a.NET_WM_PID, xproto.AtomCardinal, 0, 1).Reply()
	if err != nil || r.Format != 32 || r.ValueLen < 1 {
		return 0
	}
	return int32(xgbGet32(r.Value))
}

// processDir is the working directory of the process pid's first child,
// as a terminal's shell, or of the process itself when it has none, ""
// when it can't be told.
func processDir(pid int32) string {
	p, err := process.NewProcess(pid)
	if err != nil {
		return ""
	}
	if kids, err := p.Children(); err == nil && len(kids) > 0 {
		sort.Slice(kids, func(i, j int) bool { return kids[i].Pid < kids[j].Pid })
		p = kids[0]
	}
	dir, err := p.Cwd()
	if err != nil {
		return ""
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// focusedDir is the directory of the active tab's shell, "" when unknown.
func (wm *Manager) focusedDir() string {
	win, client := wm.focusTarget()
	if !client {
		return ""
	}
	if pid := wm.windowPID(win); pid > 0 {
		return processDir(pid)
	}
	return ""
}

// spawnTerminal starts an xterm, which opens in the active frame, in the
// colors installXtermTheme set up, in the directory of the terminal with
// the focus.
func (wm *Manager) spawnTerminal() {
	cmd := exec.Command("xterm")
	cmd.Dir = wm.focusedDir()
	wm.start(cmd)
}
