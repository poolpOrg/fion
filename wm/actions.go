package wm

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The actions are what fion does, by name, with their arguments: what the
// bindings do, and a little more. fion ctl do runs them, for scripts and
// editors, and fion ctl actions lists them.

type action struct {
	name string
	args string // what it takes, for the list: "" when nothing
	desc string
	run  func(wm *Manager, args []string) error
}

var sides = "left|right|up|down"

// side reads a side, as the bindings' arrows.
func side(s string) (direction, error) {
	if d, ok := sideNames[strings.ToLower(s)]; ok {
		return d, nil
	}
	return 0, fmt.Errorf("%q is not a side: %s", s, sides)
}

// sideAnd reads a side and a number of pixels, def when not given.
func sideAnd(args []string, def int) (direction, int, error) {
	if len(args) < 1 || len(args) > 2 {
		return 0, 0, fmt.Errorf("takes a side, %s, and pixels", sides)
	}
	d, err := side(args[0])
	if err != nil {
		return 0, 0, err
	}
	n := def
	if len(args) == 2 {
		if n, err = strconv.Atoi(strings.TrimSuffix(args[1], "px")); err != nil {
			return 0, 0, fmt.Errorf("%q is not a number of pixels", args[1])
		}
	}
	return d, n, nil
}

func noArgs(f func(wm *Manager) error) func(*Manager, []string) error {
	return func(wm *Manager, args []string) error {
		if len(args) != 0 {
			return errors.New("takes no argument")
		}
		return f(wm)
	}
}

// onOff reads on, off or toggle, nothing being toggle, against the state
// now: it reports whether to change it.
func onOff(args []string, now bool) (bool, error) {
	if len(args) == 0 {
		return true, nil
	}
	if len(args) > 1 {
		return false, errors.New("takes on, off or toggle")
	}
	switch args[0] {
	case "on", "show":
		return !now, nil
	case "off", "hide":
		return now, nil
	case "toggle":
		return true, nil
	}
	return false, fmt.Errorf("%q: on, off or toggle", args[0])
}

var actions = []action{
	{"next-tab", "", "select the next tab", noArgs(func(wm *Manager) error { wm.GetActiveFrame().cycleClientRight(); return nil })},
	{"prev-tab", "", "select the previous tab", noArgs(func(wm *Manager) error { wm.GetActiveFrame().cycleClientLeft(); return nil })},
	{"terminal", "[dir]", "open a terminal, in dir or that of the one with the focus", func(wm *Manager, args []string) error {
		if len(args) > 1 {
			return errors.New("takes a directory at most")
		}
		cmd := exec.Command("xterm")
		cmd.Dir = wm.focusedDir()
		if len(args) == 1 {
			cmd.Dir = expandHome(args[0])
		}
		wm.start(cmd)
		return nil
	}},
	{"focus", sides, "go to the frame on that side, on the next monitor past the edge", func(wm *Manager, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("takes a side, %s", sides)
		}
		d, err := side(args[0])
		if err != nil {
			return err
		}
		return wm.focusFrame(d)
	}},
	{"split", sides, "split the active frame, the new one on that side", func(wm *Manager, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("takes a side, %s", sides)
		}
		d, err := side(args[0])
		if err != nil {
			return err
		}
		return wm.GetActiveFrame().splitTowards(d)
	}},
	{"move-tab", sides, "move the active tab to the frame on that side", func(wm *Manager, args []string) error {
		if len(args) != 1 {
			return fmt.Errorf("takes a side, %s", sides)
		}
		d, err := side(args[0])
		if err != nil {
			return err
		}
		return wm.moveTab(d)
	}},
	{"grow", sides + " [pixels]", "move the active frame's edge on that side outward, 100 pixels by default", func(wm *Manager, args []string) error {
		d, n, err := sideAnd(args, 100)
		if err != nil {
			return err
		}
		return wm.resizeActive(d, n)
	}},
	{"shrink", sides + " [pixels]", "move the active frame's edge on that side inward, 100 pixels by default", func(wm *Manager, args []string) error {
		d, n, err := sideAnd(args, 100)
		if err != nil {
			return err
		}
		return wm.resizeActive(d, -n)
	}},
	{"fullscreen", "[on|off|toggle]", "show the active tab full screen, or back", func(wm *Manager, args []string) error {
		s := wm.GetActiveScreen()
		change, err := onOff(args, s.fullscreen.client != 0)
		if err != nil || !change {
			return err
		}
		return s.toggleFullscreen()
	}},
	{"scratchpad", "[show|hide|toggle]", "show or hide the scratchpad", func(wm *Manager, args []string) error {
		s := wm.GetActiveScreen()
		change, err := onOff(args, s.scratchpadShown)
		if err != nil || !change {
			return err
		}
		return s.toggleScratchpad()
	}},
	{"move-scratchpad", sides + " [pixels]", "move the scratchpad, 100 pixels by default", func(wm *Manager, args []string) error {
		d, n, err := sideAnd(args, 100)
		if err != nil {
			return err
		}
		f := wm.GetActiveFrame()
		if !f.floating() {
			return errors.New("the scratchpad isn't shown")
		}
		f.moveEdges(d, n, true)
		saveScratchpad(f.screen.onMonitor(f.g))
		return nil
	}},
	{"workspace", "next|prev|new|N", "go to a workspace of the monitor with the focus, or make one", func(wm *Manager, args []string) error {
		if len(args) != 1 {
			return errors.New("takes next, prev, new or a number")
		}
		switch a := args[0]; a {
		case "next":
			wm.switchWorkspace(1)
		case "prev", "previous":
			wm.switchWorkspace(-1)
		case "new":
			return wm.createWorkspace()
		default:
			n, err := strconv.Atoi(a)
			s := wm.GetActiveScreen()
			if err != nil || n < 1 || n > len(s.Workspaces) {
				return fmt.Errorf("no workspace %q", a)
			}
			s.showWorkspace(s.Workspaces[n-1])
		}
		wm.GetActiveScreen().updateTitleBars()
		return nil
	}},
	{"monitor", "next|prev|N", "give the focus to another monitor", func(wm *Manager, args []string) error {
		if len(args) != 1 {
			return errors.New("takes next, prev or a number")
		}
		i, n := wm.active, len(wm.Screens)
		switch a := args[0]; a {
		case "next":
			i = (i + 1) % n
		case "prev", "previous":
			i = (i + n - 1) % n
		default:
			m, err := strconv.Atoi(a)
			if err != nil || m < 1 || m > n {
				return fmt.Errorf("no monitor %q", a)
			}
			i = m - 1
		}
		wm.setActiveScreen(wm.Screens[i])
		return nil
	}},
	{"close", "", "close what has the focus, asking first", noArgs(func(wm *Manager) error { return wm.requestClose() })},
	{"panel", "[show|hide|toggle|VIEW]", "show or hide the system panel, or show one of its views: " + strings.ToLower(strings.Join(panelViews, ", ")), func(wm *Manager, args []string) error {
		s := wm.GetActiveScreen()
		shown := s.panel != nil && s.panel.shown
		if len(args) == 1 {
			if i := slices.IndexFunc(panelViews, func(v string) bool { return strings.EqualFold(v, args[0]) }); i >= 0 {
				if !shown {
					if err := s.togglePanel(); err != nil {
						return err
					}
				}
				s.panel.view = i
				s.panel.gathered = s.panel.gathered.AddDate(-1, 0, 0)
				s.panel.gather()
				s.panel.draw()
				return nil
			}
		}
		change, err := onOff(args, shown)
		if err != nil || !change {
			return err
		}
		return s.togglePanel()
	}},
	{"launch", "COMMAND...", "run a command line, through the shell", func(wm *Manager, args []string) error {
		if len(args) == 0 {
			return errors.New("takes a command line")
		}
		wm.spawn("/bin/sh", "-c", strings.Join(args, " "))
		return nil
	}},
	{"project", "NAME|DIR", "open a project, by its name or directory, as a workspace", func(wm *Manager, args []string) error {
		if len(args) != 1 {
			return errors.New("takes a project's name or directory")
		}
		dir, err := findProject(args[0])
		if err != nil {
			return err
		}
		return wm.openProject(dir)
	}},
	{"screenshot", "tab|frame|workspace", "save a screenshot", func(wm *Manager, args []string) error {
		if len(args) != 1 {
			return errors.New("takes tab, frame or workspace")
		}
		tab, hasTab, frame, workspace := wm.captureTargets()
		switch args[0] {
		case "tab":
			if !hasTab {
				return errors.New("no tab")
			}
			wm.capture(tab, false, false)
		case "frame":
			wm.capture(frame, false, false)
		case "workspace", "screen":
			wm.capture(workspace, false, false)
		default:
			return fmt.Errorf("%q: tab, frame or workspace", args[0])
		}
		return nil
	}},
	{"clear-messages", "", "dismiss the messages", noArgs(func(wm *Manager) error { wm.clearNotifications(); return nil })},
	{"cheat-sheet", "", "show the key bindings", noArgs(func(wm *Manager) error { return wm.showCheatSheet() })},
}

func findAction(name string) (action, bool) {
	for _, a := range actions {
		if a.name == name {
			return a, true
		}
	}
	return action{}, false
}

// actionList lists the actions, one a line: name, arguments, what it does.
func actionList() string {
	var b strings.Builder
	for _, a := range actions {
		line := a.name
		if a.args != "" {
			line += " " + a.args
		}
		fmt.Fprintf(&b, "%-36s %s\n", line, a.desc)
	}
	return b.String()
}

// parseAction splits an action's line into its name and arguments, and
// checks the name.
func parseAction(line string) (action, []string, error) {
	f := strings.Fields(line)
	if len(f) == 0 {
		return action{}, nil, errors.New("no action")
	}
	a, ok := findAction(strings.ToLower(f[0]))
	if !ok {
		return action{}, nil, fmt.Errorf("no action %q", f[0])
	}
	return a, f[1:], nil
}

// runAction runs an action, as a binding would: a tab shown full screen or
// a dialog with the focus gives it back first, but to the actions on
// them.
func (wm *Manager) runAction(a action, args []string) error {
	if a.name != "fullscreen" {
		wm.GetActiveScreen().leaveFullscreen()
	}
	if a.name != "close" {
		wm.dialogFocus = 0
	}
	err := a.run(wm, args)
	wm.updateFocus()
	return err
}

// resizeActive moves the active frame's edge on the side d by delta
// pixels, outward, inward when negative.
func (wm *Manager) resizeActive(d direction, delta int) error {
	f := wm.GetActiveFrame()
	if !f.resize(d, delta) {
		return errors.New("no edge to move on that side")
	}
	f.screen.updateTitleBars()
	if f.floating() {
		saveScratchpad(f.screen.onMonitor(f.g))
	}
	return nil
}

// findProject returns the directory of the project named, or the
// directory given.
func findProject(name string) (string, error) {
	if dir := expandHome(name); strings.ContainsRune(name, filepath.Separator) {
		abs, err := filepath.Abs(dir)
		return abs, err
	}
	var found []string
	for _, d := range findProjects(projectRoots()) {
		if strings.EqualFold(filepath.Base(d), name) {
			found = append(found, d)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no project %q", name)
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("%d projects named %q: give its directory", len(found), name)
}
