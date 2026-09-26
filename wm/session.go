package wm

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jezek/xgb/xproto"
)

// Mod+Escape, or the button at the end of the bar, offers to restart
// fion, log out or quit.
//
// Restarting keeps the layout: fion writes it down, the monitors'
// workspaces, their frames and the windows in each, and executes itself
// again, the binary installed now. X hands the windows back to the root
// as fion's connection closes, and the new fion puts each back where it
// was, by its id.
//
// Logging out asks every window to close, and quits once they did, which
// ends the session when fion is the last command of .xinitrc. A window
// still open after a while, asking whether to save, cancels it: logging
// out again closes the windows left by force.

// Restart is what Run returns when fion is to be executed again, the
// layout written in Path.
type Restart struct{ Path string }

func (r *Restart) Error() string { return "restart" }

type exitKind int

const (
	exitNone exitKind = iota
	exitQuit
	exitRestart
)

// how long logging out waits for the windows to close
const logoutWait = 10 * time.Second

// sessionMenu offers to restart, log out or quit.
func (wm *Manager) sessionMenu() error {
	if _, err := wm.showPrompt("Session: r restarts fion, l logs out, q quits fion, any other key cancels", colorAccent); err != nil {
		return err
	}
	wm.mode = &keyMode{key: func(sym xproto.Keysym) bool {
		// once the prompt is gone
		switch sym {
		case XK_r:
			wm.after(0, func() { wm.restart() })
		case XK_l:
			wm.after(0, func() { wm.logout() })
		case XK_q:
			wm.exiting = exitQuit
		}
		return true
	}}
	return nil
}

// restart writes the layout down, and has Run return for fion to be
// executed again.
func (wm *Manager) restart() {
	path := filepath.Join(stateDir(), "restart-"+strings.NewReplacer("/", "_", ":", "_").Replace(os.Getenv("DISPLAY"))+".json")
	b, err := json.Marshal(wm.layout())
	if err == nil {
		if err = os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
			err = os.WriteFile(path, b, 0o600)
		}
	}
	if err != nil {
		wm.alert("Can't restart: " + err.Error())
		return
	}
	wm.restartPath = path
	wm.exiting = exitRestart
}

// logout asks the windows to close, and quits once they did.
func (wm *Manager) logout() {
	n := len(wm.Clients) + len(wm.dialogs)
	if n == 0 {
		wm.exiting = exitQuit
		return
	}
	for win := range wm.dialogs {
		wm.closeDialog(win)
	}
	for win := range wm.Clients {
		wm.closeClient(win)
	}
	wm.logoutUntil = time.Now().Add(logoutWait)
	wm.notify(levelInfo, fmt.Sprintf("logging out: closing %d windows", n), logoutWait)
}

// checkLogout quits once the windows closed, or gives up, every second.
func (wm *Manager) checkLogout() {
	if wm.logoutUntil.IsZero() {
		return
	}
	left := len(wm.Clients) + len(wm.dialogs)
	switch {
	case left == 0:
		wm.exiting = exitQuit
	case time.Now().After(wm.logoutUntil):
		wm.logoutUntil = time.Time{}
		wm.clearNotifications()
		wm.notify(levelWarn, fmt.Sprintf("logout canceled: %d windows didn't close; log out again to close them by force", left), 0)
	}
}

// The layout, as written down on restart.

type savedFrame struct {
	Vertical     bool          `json:"vertical,omitempty"`
	Ratio        float64       `json:"ratio,omitempty"`
	Children     []*savedFrame `json:"children,omitempty"`
	Clients      []uint32      `json:"clients,omitempty"`
	ActiveClient int           `json:"active_client,omitempty"`
	Active       bool          `json:"active,omitempty"`
}

type savedWorkspace struct {
	Name string      `json:"name,omitempty"`
	Root *savedFrame `json:"root"`
}

type savedScreen struct {
	Monitor    string           `json:"monitor"`
	Workspaces []savedWorkspace `json:"workspaces"`
	Active     int              `json:"active"`
	Scratchpad *savedFrame      `json:"scratchpad,omitempty"`
	Shown      bool             `json:"scratchpad_shown,omitempty"`
	Fullscreen uint32           `json:"fullscreen,omitempty"`
}

type savedLayout struct {
	Screens []savedScreen `json:"screens"`
	Active  int           `json:"active"`
}

func saveFrame(f *Frame, active *Frame) *savedFrame {
	sf := &savedFrame{Active: f == active}
	if !f.leaf {
		sf.Vertical, sf.Ratio = f.vertical, f.ratio
		for _, c := range f.children {
			sf.Children = append(sf.Children, saveFrame(c, active))
		}
		return sf
	}
	for _, w := range f.clients {
		sf.Clients = append(sf.Clients, uint32(w))
	}
	sf.ActiveClient = max(0, f.activeClient)
	return sf
}

// layout is the layout now.
func (wm *Manager) layout() savedLayout {
	l := savedLayout{Active: wm.active}
	for _, s := range wm.Screens {
		ss := savedScreen{Monitor: s.monitor.name, Active: s.activeWorkspaceIdx, Fullscreen: uint32(s.fullscreen.client)}
		for _, ws := range s.Workspaces {
			ss.Workspaces = append(ss.Workspaces, savedWorkspace{Name: ws.name, Root: saveFrame(ws.Root, ws.ActiveFrame)})
		}
		if s.scratchpad != nil {
			ss.Scratchpad, ss.Shown = saveFrame(s.scratchpad, nil), s.scratchpadShown
		}
		l.Screens = append(l.Screens, ss)
	}
	return l
}

// restoring is a frame waiting for its windows, in their order.
type restoring struct {
	frame   *Frame
	clients []xproto.Window
	active  int
}

// loadLayout reads the layout written at path, and removes it.
func loadLayout(path string) (savedLayout, error) {
	var l savedLayout
	b, err := os.ReadFile(path)
	os.Remove(path)
	if err != nil {
		return l, err
	}
	return l, json.Unmarshal(b, &l)
}

// prepareLayout rebuilds the workspaces and frames of l, for the windows
// adopted next to go back in theirs.
func (wm *Manager) prepareLayout(l savedLayout) {
	wm.restored = map[xproto.Window]*Frame{}
	used := map[*Screen]bool{}
	for i, ss := range l.Screens {
		// the monitor by name, or place
		var s *Screen
		for _, o := range wm.Screens {
			if !used[o] && o.monitor.name == ss.Monitor {
				s = o
			}
		}
		if s == nil && i < len(wm.Screens) && !used[wm.Screens[i]] {
			s = wm.Screens[i]
		}
		if s == nil {
			// a monitor gone: its windows go where they are adopted
			continue
		}
		used[s] = true
		for j, sw := range ss.Workspaces {
			ws := s.Workspaces[0]
			if j > 0 {
				var err error
				if ws, err = newWorkspace(s); err != nil {
					log.Printf("restoring a workspace: %v", err)
					continue
				}
				s.Workspaces = append(s.Workspaces, ws)
			}
			ws.name = sw.Name
			if sw.Root != nil {
				// once built, as splitting makes the new frames active
				if active := wm.buildFrames(ws.Root, sw.Root); active != nil {
					ws.ActiveFrame = active
				}
			}
		}
		if ss.Scratchpad != nil && len(ss.Scratchpad.Clients) > 0 {
			// created hidden, not to take the other windows adopted, and
			// shown at the end if it was
			if err := s.toggleScratchpad(); err == nil {
				s.toggleScratchpad()
				wm.expect(s.scratchpad, ss.Scratchpad)
			}
		}
	}
	wm.pendingLayout = &l
}

// buildFrames splits f as sf was, and has the windows of its leaves
// expected there, a frame too small to split taking all of them, and
// returns the leaf that was active, nil when none.
func (wm *Manager) buildFrames(f *Frame, sf *savedFrame) *Frame {
	if len(sf.Children) == 2 {
		if err := f.split(sf.Vertical, false); err == nil {
			f.ratio = sf.Ratio
			f.layout()
			a := wm.buildFrames(f.children[0], sf.Children[0])
			if b := wm.buildFrames(f.children[1], sf.Children[1]); b != nil {
				a = b
			}
			return a
		}
		sf = flatten(sf)
	}
	wm.expect(f, sf)
	if sf.Active {
		return f
	}
	return nil
}

// flatten gathers the windows of sf's leaves into one.
func flatten(sf *savedFrame) *savedFrame {
	out := &savedFrame{Active: sf.Active}
	var walkSaved func(*savedFrame)
	walkSaved = func(s *savedFrame) {
		out.Clients = append(out.Clients, s.Clients...)
		out.Active = out.Active || s.Active
		for _, c := range s.Children {
			walkSaved(c)
		}
	}
	walkSaved(sf)
	return out
}

func (wm *Manager) expect(f *Frame, sf *savedFrame) {
	r := restoring{frame: f, active: sf.ActiveClient}
	for _, c := range sf.Clients {
		win := xproto.Window(c)
		wm.restored[win] = f
		r.clients = append(r.clients, win)
	}
	wm.restoring = append(wm.restoring, r)
}

// finishLayout puts the windows adopted in their order, with the tabs,
// frames, workspaces, scratchpads and monitor that were active.
func (wm *Manager) finishLayout() {
	l := wm.pendingLayout
	if l == nil {
		return
	}
	wm.pendingLayout, wm.restored = nil, nil
	for _, r := range wm.restoring {
		f := r.frame
		// in their order, those that came back
		sorted := slices.DeleteFunc(slices.Clone(r.clients), func(w xproto.Window) bool { return !slices.Contains(f.clients, w) })
		for _, w := range f.clients {
			if !slices.Contains(sorted, w) {
				sorted = append(sorted, w)
			}
		}
		f.clients = sorted
		if len(f.clients) > 0 {
			active := 0
			if r.active < len(r.clients) {
				active = max(0, slices.Index(f.clients, r.clients[r.active]))
			}
			f.selectClient(active)
		}
	}
	wm.restoring = nil
	for i, ss := range l.Screens {
		if i >= len(wm.Screens) {
			break
		}
		s := wm.Screens[i]
		for _, o := range wm.Screens {
			if o.monitor.name == ss.Monitor {
				s = o
			}
		}
		if ss.Active < len(s.Workspaces) {
			s.showWorkspace(s.Workspaces[ss.Active])
		}
		if s.scratchpad != nil && s.scratchpadShown != ss.Shown {
			s.toggleScratchpad()
		}
		if c, ok := wm.Clients[xproto.Window(ss.Fullscreen)]; ok && ss.Fullscreen != 0 {
			wm.activate(xproto.Window(ss.Fullscreen))
			c.frame.screen.toggleFullscreen()
		}
		s.updateTitleBars()
		for _, ws := range s.Workspaces {
			ws.updateInfoBar()
		}
	}
	if l.Active < len(wm.Screens) {
		wm.setActiveScreen(wm.Screens[l.Active])
	}
	wm.updateFocus()
}
