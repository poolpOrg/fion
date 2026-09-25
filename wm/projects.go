package wm

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jezek/xgb/xproto"
)

// Projects: the launcher lists the git repositories under the usual
// places, or those FION_PROJECTS names, separated by colons, and opens one
// as a workspace named after it, with a terminal in it, or what the
// project's .fion file lays out:
//
//	# a terminal on the left, the editor on the right, a shell below it
//	xterm
//	split right
//	xterm -e nvim .
//	split down
//	xterm
//
// Each line runs in the project's directory, its window opening in the
// frame active when it ran; split and focus take a side as the bindings
// do.

const projectPrefix = "project:"

// projectRoots are the directories searched for projects.
func projectRoots() []string {
	if v := os.Getenv("FION_PROJECTS"); v != "" {
		return filepath.SplitList(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var roots []string
	for _, d := range []string{"src", "code", "projects", "Wip", "git", "dev", "go/src"} {
		roots = append(roots, filepath.Join(home, d))
	}
	return roots
}

// findProjects returns the git repositories under roots, up to four levels
// down, as ~/go/src/github.com/owner/repo is, not looking into them.
func findProjects(roots []string) []string {
	seen := map[string]bool{}
	var out []string
	const maxDepth, maxVisited = 4, 50000
	visited := 0
	for _, root := range roots {
		root = filepath.Clean(root)
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if visited++; visited > maxVisited {
				return filepath.SkipAll
			}
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
				if !seen[path] {
					seen[path] = true
					out = append(out, path)
				}
				return filepath.SkipDir
			}
			if rel, _ := filepath.Rel(root, path); strings.Count(rel, string(filepath.Separator)) >= maxDepth-1 && rel != "." {
				return filepath.SkipDir
			}
			return nil
		})
	}
	sort.Strings(out)
	return out
}

// projectItems are the launcher's entries for the projects.
func projectItems(dirs []string) []launchItem {
	home, _ := os.UserHomeDir()
	var items []launchItem
	for _, d := range dirs {
		short := d
		if home != "" && strings.HasPrefix(d, home+string(filepath.Separator)) {
			short = "~" + d[len(home):]
		}
		items = append(items, launchItem{label: filepath.Base(d), command: projectPrefix + short, kind: kindProject})
	}
	return items
}

// projectDir is the directory a project's command line names.
func projectDir(line string) (string, bool) {
	dir, ok := strings.CutPrefix(line, projectPrefix)
	if !ok {
		return "", false
	}
	return expandHome(dir), true
}

// expandHome replaces a leading ~ with the home directory.
func expandHome(path string) string {
	if rest, ok := strings.CutPrefix(path, "~"); ok && (rest == "" || rest[0] == filepath.Separator) {
		if home, err := os.UserHomeDir(); err == nil {
			return home + rest
		}
	}
	return path
}

// layoutStep is a line of a project's .fion: split or focus towards a
// side, or run a command.
type layoutStep struct {
	split, focus bool
	dir          direction
	command      string
}

var sideNames = map[string]direction{"left": dirLeft, "right": dirRight, "up": dirUp, "above": dirUp, "down": dirDown, "below": dirDown}

func parseLayout(r io.Reader) ([]layoutStep, error) {
	var steps []layoutStep
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		verb, arg, _ := strings.Cut(line, " ")
		if verb == "split" || verb == "focus" {
			d, ok := sideNames[strings.TrimSpace(arg)]
			if !ok {
				return nil, fmt.Errorf("line %d: %s takes left, right, up or down", n, verb)
			}
			steps = append(steps, layoutStep{split: verb == "split", focus: verb == "focus", dir: d})
			continue
		}
		steps = append(steps, layoutStep{command: line})
	}
	return steps, sc.Err()
}

// openProject opens dir as a new workspace, laid out by its .fion.
func (wm *Manager) openProject(dir string) error {
	steps := []layoutStep{{command: "xterm"}}
	if f, err := os.Open(filepath.Join(dir, ".fion")); err == nil {
		steps, err = parseLayout(f)
		f.Close()
		if err != nil {
			wm.alert(".fion: " + err.Error())
			return err
		}
	}
	if err := wm.createWorkspace(); err != nil {
		return err
	}
	ws := wm.GetActiveWorkspace()
	ws.name = filepath.Base(dir)
	for _, st := range steps {
		switch {
		case st.split:
			if err := wm.GetActiveFrame().splitTowards(st.dir); err != nil {
				log.Printf("%s/.fion: %v", dir, err)
			}
		case st.focus:
			wm.focusFrame(st.dir)
		default:
			cmd := exec.Command("/bin/sh", "-c", "exec "+st.command)
			cmd.Dir = dir
			if err := cmd.Start(); err != nil {
				log.Printf("%s/.fion: %v", dir, err)
				continue
			}
			wm.placeWindowOf(int32(cmd.Process.Pid), wm.GetActiveFrame())
			go cmd.Wait()
		}
	}
	ws.updateInfoBar()
	wm.updateFocus()
	return nil
}

// A placement has the window of a process started by fion open in a
// given frame, for a while, rather than in the active one.
type placement struct {
	frame *Frame
	until time.Time
}

func (wm *Manager) placeWindowOf(pid int32, f *Frame) {
	if wm.placements == nil {
		wm.placements = map[int32]placement{}
	}
	wm.placements[pid] = placement{frame: f, until: time.Now().Add(time.Minute)}
}

// placedFrame is where the window win of a process placed goes, nil when
// it wasn't, or its frame is gone.
func (wm *Manager) placedFrame(win xproto.Window) *Frame {
	if len(wm.placements) == 0 {
		return nil
	}
	now := time.Now()
	for pid, p := range wm.placements {
		if now.After(p.until) {
			delete(wm.placements, pid)
		}
	}
	pid := wm.windowPID(win)
	p, ok := wm.placements[pid]
	if !ok {
		return nil
	}
	delete(wm.placements, pid)
	f := p.frame
	if f.floating() {
		return f
	}
	alive := false
	walk(f.workspace.Root, func(c *Frame) { alive = alive || c == f })
	if !alive || !f.leaf {
		return nil
	}
	return f
}
