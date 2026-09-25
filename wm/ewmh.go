package wm

import (
	"log"
	"slices"

	"github.com/jezek/xgb/xproto"
)

// What fion does of EWMH, beyond announcing itself: the list of the
// windows it manages, the one with the focus, full screen asked by the
// windows themselves, and the requests to activate or close a window.

// supportedAtoms are the hints fion follows.
func (s *Screen) supportedAtoms() []xproto.Atom {
	a := s.atoms
	return []xproto.Atom{
		a.NET_SUPPORTED, a.NET_SUPPORTING_WM_CHECK, a.NET_CLIENT_LIST, a.NET_ACTIVE_WINDOW,
		a.NET_WM_NAME, a.NET_WM_STATE, a.NET_WM_STATE_FULLSCREEN, a.NET_WM_STATE_DEMANDS_ATTENTION,
		a.NET_WM_WINDOW_TYPE, a.NET_WM_WINDOW_TYPE_DOCK, a.NET_WM_WINDOW_TYPE_DIALOG,
		a.NET_WM_WINDOW_TYPE_UTILITY, a.NET_WM_WINDOW_TYPE_SPLASH, a.NET_WM_WINDOW_TYPE_TOOLBAR,
		a.NET_WM_WINDOW_TYPE_NOTIFICATION, a.NET_WM_WINDOW_TYPE_DESKTOP, a.NET_CLOSE_WINDOW, a.NET_WM_PID,
	}
}

// updateClientList sets _NET_CLIENT_LIST, the windows managed, oldest
// first as EWMH asks, which fion approximates by window id.
func (wm *Manager) updateClientList() {
	var wins []xproto.Window
	for w := range wm.Clients {
		wins = append(wins, w)
	}
	for w := range wm.dialogs {
		wins = append(wins, w)
	}
	slices.Sort(wins)
	buf := make([]byte, 4*len(wins))
	for i, w := range wins {
		buf[4*i], buf[4*i+1], buf[4*i+2], buf[4*i+3] = byte(w), byte(w>>8), byte(w>>16), byte(w>>24)
	}
	s := wm.Screens[0]
	xproto.ChangeProperty(wm.Conn(), xproto.PropModeReplace, s.Info().Root, s.atoms.NET_CLIENT_LIST,
		xproto.AtomWindow, 32, uint32(len(wins)), buf)
}

// setFullscreenState tells win whether it is shown full screen.
func (wm *Manager) setFullscreenState(win xproto.Window, full bool) {
	a := wm.Screens[0].atoms
	state := slices.DeleteFunc(wm.atomList(win, a.NET_WM_STATE), func(x xproto.Atom) bool { return x == a.NET_WM_STATE_FULLSCREEN })
	if full {
		state = append(state, a.NET_WM_STATE_FULLSCREEN)
	}
	buf := make([]byte, 4*len(state))
	for i, x := range state {
		buf[4*i], buf[4*i+1], buf[4*i+2], buf[4*i+3] = byte(x), byte(x>>8), byte(x>>16), byte(x>>24)
	}
	xproto.ChangeProperty(wm.Conn(), xproto.PropModeReplace, win, a.NET_WM_STATE, xproto.AtomAtom, 32, uint32(len(state)), buf)
}

// wantsFullscreen reports whether win asks, in its _NET_WM_STATE, to be
// shown full screen.
func (wm *Manager) wantsFullscreen(win xproto.Window) bool {
	a := wm.Screens[0].atoms
	return slices.Contains(wm.atomList(win, a.NET_WM_STATE), a.NET_WM_STATE_FULLSCREEN)
}

// activate shows the client win: its monitor, workspace, frame and tab
// made the active ones.
func (wm *Manager) activate(win xproto.Window) {
	if d, ok := wm.dialogs[win]; ok {
		d.ws.Screen.showWorkspace(d.ws)
		wm.setActiveScreen(d.ws.Screen)
		wm.dialogFocus = win
		wm.updateFocus()
		return
	}
	c, ok := wm.Clients[win]
	if !ok {
		return
	}
	f := c.frame
	s := f.screen
	if !f.floating() {
		s.showWorkspace(f.workspace)
		f.workspace.ActiveFrame = f
	} else if !s.scratchpadShown {
		s.toggleScratchpad()
	}
	wm.setActiveScreen(s)
	if i := slices.Index(f.clients, win); i >= 0 {
		f.selectClient(i)
	}
	s.updateTitleBars()
	wm.updateFocus()
}

// showWorkspace shows ws on its monitor.
func (s *Screen) showWorkspace(ws *Workspace) {
	old := s.GetActiveWorkspace()
	i := slices.Index(s.Workspaces, ws)
	if i < 0 || old == ws {
		return
	}
	s.activeWorkspaceIdx = i
	ws.Map()
	old.Unmap()
}

// handleClientMessage follows the EWMH requests of the windows, and of
// pagers and tools such as xdotool.
func (wm *Manager) handleClientMessage(ev xproto.ClientMessageEvent) {
	a := wm.Screens[0].atoms
	data := ev.Data.Data32
	switch ev.Type {
	case a.NET_WM_STATE:
		c, ok := wm.Clients[ev.Window]
		if !ok {
			return
		}
		if xproto.Atom(data[1]) == a.NET_WM_STATE_DEMANDS_ATTENTION || xproto.Atom(data[2]) == a.NET_WM_STATE_DEMANDS_ATTENTION {
			switch data[0] {
			case 0:
				wm.setUrgent(ev.Window, false)
			case 1:
				wm.setUrgent(ev.Window, true)
			case 2:
				wm.setUrgent(ev.Window, !c.urgent)
			}
		}
		if xproto.Atom(data[1]) != a.NET_WM_STATE_FULLSCREEN && xproto.Atom(data[2]) != a.NET_WM_STATE_FULLSCREEN {
			return
		}
		s := c.frame.screen
		full := s.fullscreen.client == ev.Window
		want := full
		switch data[0] {
		case 0:
			want = false
		case 1:
			want = true
		case 2:
			want = !full
		}
		if want == full {
			return
		}
		if want {
			wm.activate(ev.Window)
			s.leaveFullscreen()
			if err := s.toggleFullscreen(); err != nil {
				log.Printf("full screen: %v", err)
			}
		} else {
			s.leaveFullscreen()
		}
	case a.NET_ACTIVE_WINDOW:
		wm.activate(ev.Window)
	case a.NET_CLOSE_WINDOW:
		if _, ok := wm.dialogs[ev.Window]; ok {
			wm.closeDialog(ev.Window)
		} else {
			wm.closeClient(ev.Window)
		}
	}
}
