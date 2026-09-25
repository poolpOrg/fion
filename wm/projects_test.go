package wm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func TestFindProjects(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{
		"github.com/owner/repo/.git", "github.com/owner/repo/sub/.git", // not looked into
		"plain/.git", "not-a-repo/src", ".hidden/x/.git", "a/b/c/d/e/.git", // too deep
		"node_modules/pkg/.git",
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := findProjects([]string{root, filepath.Join(root, "missing")})
	for i := range got {
		got[i], _ = filepath.Rel(root, got[i])
	}
	if strings.Join(got, " ") != "github.com/owner/repo plain" {
		t.Fatalf("projects %v", got)
	}

	items := projectItems([]string{filepath.Join(root, "plain")})
	if items[0].label != "plain" || items[0].kind != kindProject {
		t.Fatalf("items %+v", items)
	}
	if dir, ok := projectDir(items[0].command); !ok || dir != filepath.Join(root, "plain") {
		t.Fatalf("projectDir(%q) = %q", items[0].command, dir)
	}
	// counted once with its history
	merged := launchItems(nil, items, nil, map[string]int{items[0].command: 3})
	if len(merged) != 1 || merged[0].uses != 3 {
		t.Fatalf("merged %+v", merged)
	}
}

func TestParseLayout(t *testing.T) {
	steps, err := parseLayout(strings.NewReader("# comment\nxterm\n\nsplit right\nxterm -e nvim .\nfocus left\n"))
	if err != nil || len(steps) != 4 || steps[0].command != "xterm" || !steps[1].split || steps[1].dir != dirRight ||
		steps[2].command != "xterm -e nvim ." || !steps[3].focus || steps[3].dir != dirLeft {
		t.Fatalf("steps %+v, %v", steps, err)
	}
	if _, err := parseLayout(strings.NewReader("split sideways\n")); err == nil {
		t.Fatalf("a bad side parsed")
	}
}

func TestOpenProjectAndPlacements(t *testing.T) {
	wm := newTestManager(t)
	s := wm.GetActiveScreen()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".fion"), []byte("split right\nsplit down\nfocus up\n"), 0o644)
	if err := wm.openProject(dir); err != nil {
		t.Fatal(err)
	}
	ws := s.GetActiveWorkspace()
	if len(s.Workspaces) != 2 || ws.name != filepath.Base(dir) || ws.Root.leaf || ws.Root.children[1].leaf {
		t.Fatalf("project workspace %q, %d workspaces", ws.name, len(s.Workspaces))
	}
	if st := ws.Manager.GetActiveFrame(); st != ws.Root.children[1].children[0] {
		t.Fatalf("focus up didn't reach the top right frame")
	}

	// a window of a process placed opens in its frame, not the active one
	target := ws.Root.children[0]
	wm.placeWindowOf(4242, target)
	win := newTestWindow(t, wm, 100, 100, func(conn *xgb.Conn, w xproto.Window) {
		setWindowProp(conn, w, s.atoms.NET_WM_PID, xproto.AtomCardinal, 4242)
	})
	wm.manage(win, false)
	drainEvents(t, wm)
	if wm.Clients[win].frame != target {
		t.Fatalf("the window placed opened in another frame")
	}
	if _, left := wm.placements[4242]; left {
		t.Fatalf("the placement was kept once used")
	}
}
