package wm

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jezek/xgb/xproto"
)

// A window asking for attention, with the urgency hint of its WM_HINTS, as
// xterm's bell sets it, or EWMH's _NET_WM_STATE_DEMANDS_ATTENTION, has its
// tab in red, and the bar of its monitor lists the workspaces holding
// one, until it has the focus.

// setUrgent marks, or unmarks, the client win as asking for attention;
// the one with the focus never is.
func (wm *Manager) setUrgent(win xproto.Window, urgent bool) {
	c, ok := wm.Clients[win]
	if !ok {
		return
	}
	if focus, client := wm.focusTarget(); client && focus == win {
		urgent = false
	}
	if c.urgent == urgent {
		return
	}
	c.urgent = urgent
	c.frame.updateTitleBar()
	for _, ws := range c.frame.screen.Workspaces {
		ws.updateInfoBar()
	}
}

// urgencyChanged reads win's WM_HINTS again, as they changed.
func (wm *Manager) urgencyChanged(win xproto.Window) {
	h, ok := readWMHints(wm, win)
	wm.setUrgent(win, ok && h.flags&hintUrgency != 0)
}

// urgentSummary lists the workspaces of s holding windows asking for
// attention, and the scratchpad, as "! 2 3 sp", "" when none.
func (s *Screen) urgentSummary() string {
	urgent := func(f *Frame) bool {
		return slices.ContainsFunc(f.clients, func(w xproto.Window) bool {
			c, ok := s.wm.Clients[w]
			return ok && c.urgent
		})
	}
	var parts []string
	for i, ws := range s.Workspaces {
		found := false
		walk(ws.Root, func(f *Frame) { found = found || urgent(f) })
		if found {
			parts = append(parts, fmt.Sprint(i+1))
		}
	}
	if s.scratchpad != nil && urgent(s.scratchpad) {
		parts = append(parts, "sp")
	}
	if len(parts) == 0 {
		return ""
	}
	return "! " + strings.Join(parts, " ")
}
