package wm

import (
	"fmt"
	"log"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/randr"
	"github.com/jezek/xgb/xproto"
)

// fion gives each monitor, as RandR reports them, its own workspaces, bar,
// panel and scratchpad: a Screen. FION_MONITORS, as
// 1920x1080+0+0,1280x1024+1920+0, sets them by hand instead.

type monitor struct {
	name    string
	g       Geometry // in the root's coordinates
	primary bool
}

// queryMonitors returns the monitors of root, w×h, from left to right: the
// whole root when RandR, if the server has it, doesn't tell.
func queryMonitors(conn *xgb.Conn, hasRandr bool, root xproto.Window, w, h uint16) []monitor {
	if spec := os.Getenv("FION_MONITORS"); spec != "" {
		if ms, err := parseMonitors(spec); err == nil {
			return cleanMonitors(ms)
		} else {
			log.Printf("FION_MONITORS: %v", err)
		}
	}
	whole := []monitor{{name: "screen", g: Geometry{0, 0, w, h}, primary: true}}
	if !hasRandr {
		return whole
	}
	reply, err := randr.GetMonitors(conn, root, true).Reply()
	if err != nil || len(reply.Monitors) == 0 {
		return whole
	}
	var ms []monitor
	for i, m := range reply.Monitors {
		name := fmt.Sprintf("monitor%d", i)
		if a, err := xproto.GetAtomName(conn, m.Name).Reply(); err == nil {
			name = a.Name
		}
		ms = append(ms, monitor{name: name, g: Geometry{m.X, m.Y, m.Width, m.Height}, primary: m.Primary})
	}
	if ms = cleanMonitors(ms); len(ms) == 0 {
		return whole
	}
	return ms
}

// parseMonitors reads monitors as WxH+X+Y, separated by commas.
func parseMonitors(spec string) ([]monitor, error) {
	var ms []monitor
	for i, part := range strings.Split(spec, ",") {
		var w, h uint16
		var x, y int16
		if n, _ := fmt.Sscanf(strings.TrimSpace(part), "%dx%d+%d+%d", &w, &h, &x, &y); n != 4 || w == 0 || h == 0 {
			return nil, fmt.Errorf("%q is not WxH+X+Y", part)
		}
		ms = append(ms, monitor{name: fmt.Sprintf("monitor%d", i), g: Geometry{x, y, w, h}, primary: i == 0})
	}
	return ms, nil
}

// cleanMonitors drops the monitors mirroring another, and those holding
// others, as the whole screen some servers list along with the monitors,
// and sorts the rest from left to right, then top to bottom.
func cleanMonitors(ms []monitor) []monitor {
	contains := func(a, b Geometry) bool {
		return a.X <= b.X && a.Y <= b.Y &&
			int(a.X)+int(a.W) >= int(b.X)+int(b.W) && int(a.Y)+int(a.H) >= int(b.Y)+int(b.H)
	}
	var out []monitor
	for i, m := range ms {
		keep := m.g.W > 0 && m.g.H > 0
		for j, o := range ms {
			if i == j {
				continue
			}
			if m.g == o.g {
				// the first of mirrors
				if j < i {
					keep = false
				}
			} else if contains(m.g, o.g) {
				keep = false
			}
		}
		if keep {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].g.X != out[j].g.X {
			return out[i].g.X < out[j].g.X
		}
		return out[i].g.Y < out[j].g.Y
	})
	return out
}

// watchMonitors asks RandR to tell when monitors change.
func watchMonitors(conn *xgb.Conn, root xproto.Window) {
	randr.SelectInput(conn, root, randr.NotifyMaskScreenChange|randr.NotifyMaskCrtcChange|randr.NotifyMaskOutputChange)
}

// monitorsChanged has the monitors read again, once the burst of events a
// change brings is over.
func (wm *Manager) monitorsChanged() {
	if wm.monitorsPending {
		return
	}
	wm.monitorsPending = true
	wm.after(300*time.Millisecond, func() {
		wm.monitorsPending = false
		wm.updateMonitors()
	})
}

// updateMonitors follows the monitors as they are now: those still there,
// by name, or place, keep their workspaces, moved and resized, the new ones
// get a workspace of their own, and the workspaces of those gone go to
// the first monitor.
func (wm *Manager) updateMonitors() {
	info := xproto.Setup(wm.xConn).Roots[0]
	w, h := info.WidthInPixels, info.HeightInPixels
	if g, err := xproto.GetGeometry(wm.Conn(), xproto.Drawable(info.Root)).Reply(); err == nil {
		w, h = g.Width, g.Height
	}
	ms := queryMonitors(wm.Conn(), wm.hasRandr, info.Root, w, h)

	active := wm.GetActiveScreen()
	used := map[*Screen]bool{}
	match := func(same func(o *Screen) bool) *Screen {
		for _, o := range wm.Screens {
			if !used[o] && same(o) {
				used[o] = true
				return o
			}
		}
		return nil
	}
	var next []*Screen
	for _, m := range ms {
		s := match(func(o *Screen) bool { return o.monitor.name == m.name })
		if s == nil {
			s = match(func(o *Screen) bool { return o.monitor.g == m.g })
		}
		if s == nil {
			var err error
			if s, err = newScreen(wm, info, m, wm.Screens[0].atoms); err != nil {
				log.Printf("monitor %s: %v", m.name, err)
				continue
			}
			log.Printf("monitor %s added, %dx%d+%d+%d", m.name, m.g.W, m.g.H, m.g.X, m.g.Y)
		} else {
			s.setMonitor(m)
		}
		next = append(next, s)
	}
	if len(next) == 0 {
		return
	}
	for _, o := range wm.Screens {
		if !used[o] {
			log.Printf("monitor %s gone, its workspaces moved to %s", o.monitor.name, next[0].monitor.name)
			o.retire(next[0])
		}
	}
	wm.Screens = next
	wm.active = max(0, slices.Index(next, active))
	for _, s := range wm.Screens {
		s.updateTitleBars()
		for _, ws := range s.Workspaces {
			ws.updateInfoBar()
		}
	}
	wm.updateFocus()
}

// setMonitor moves and resizes the screen to the monitor m.
func (s *Screen) setMonitor(m monitor) {
	moved := s.monitor.g != m.g
	s.monitor = m
	if !moved {
		return
	}
	conn, g := s.Conn(), m.g
	mask := uint16(xproto.ConfigWindowX | xproto.ConfigWindowY | xproto.ConfigWindowWidth | xproto.ConfigWindowHeight)
	for _, ws := range s.Workspaces {
		xproto.ConfigureWindow(conn, ws.WorkspaceWindow, mask, []uint32{uint32(g.X), uint32(g.Y), uint32(g.W), uint32(g.H)})
		xproto.ConfigureWindow(conn, ws.InfoBarWindow, xproto.ConfigWindowY|xproto.ConfigWindowWidth,
			[]uint32{uint32(int(g.H) - infoBarOuterH()), uint32(g.W - 2)})
	}
	if s.panel != nil && s.panel.shown {
		// placed again, and the frames above it
		s.togglePanel()
		s.togglePanel()
	}
	s.layoutWorkspaces()
	if sp := s.scratchpad; sp != nil {
		sp.g = s.scratchpadGeometry()
		sp.layout()
	}
	if win := s.fullscreen.client; win != 0 {
		s.leaveFullscreen()
	}
}

// retire hands the workspaces of a monitor gone to the screen to, hidden
// behind its own, and the tabs of its scratchpad to to's active frame.
func (s *Screen) retire(to *Screen) {
	conn := s.Conn()
	s.leaveFullscreen()
	if s.panel != nil && s.panel.shown {
		s.togglePanel()
	}
	for _, ws := range s.Workspaces {
		ws.Screen = to
		walk(ws.Root, func(f *Frame) { f.screen = to })
		ws.Unmap()
		to.Workspaces = append(to.Workspaces, ws)
	}
	g := to.Geometry()
	mask := uint16(xproto.ConfigWindowX | xproto.ConfigWindowY | xproto.ConfigWindowWidth | xproto.ConfigWindowHeight)
	for _, ws := range to.Workspaces {
		xproto.ConfigureWindow(conn, ws.WorkspaceWindow, mask, []uint32{uint32(g.X), uint32(g.Y), uint32(g.W), uint32(g.H)})
		xproto.ConfigureWindow(conn, ws.InfoBarWindow, xproto.ConfigWindowY|xproto.ConfigWindowWidth,
			[]uint32{uint32(int(g.H) - infoBarOuterH()), uint32(g.W - 2)})
	}
	to.layoutWorkspaces()
	if sp := s.scratchpad; sp != nil {
		target := to.GetActiveWorkspace().GetActiveFrame()
		for _, win := range slices.Clone(sp.clients) {
			if err := s.wm.moveClient(win, target, -1); err != nil {
				log.Printf("scratchpad of a monitor gone: %v", err)
			}
		}
		sp.Destroy()
		s.scratchpad, s.scratchpadShown = nil, false
	}
	for _, w := range []xproto.Window{s.fullscreen.window} {
		if w != 0 {
			xproto.DestroyWindow(conn, w)
		}
	}
	if s.panel != nil {
		xproto.DestroyWindow(conn, s.panel.window)
	}
	if s.prompt != nil {
		xproto.DestroyWindow(conn, s.prompt.window)
	}
}
