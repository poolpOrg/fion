package wm

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActionList(t *testing.T) {
	seen := map[string]bool{}
	list := actionList()
	for _, a := range actions {
		if seen[a.name] || a.desc == "" || !strings.Contains(list, a.name) {
			t.Errorf("action %q: duplicate, undescribed or unlisted", a.name)
		}
		seen[a.name] = true
	}
	var out bytes.Buffer
	if ok, err := Control([]string{"ctl", "actions"}, nil, &out); !ok || err != nil || out.String() != list {
		t.Fatalf("fion ctl actions: %v %v", ok, err)
	}
	if _, _, err := parseAction("explode now"); err == nil {
		t.Fatalf("an unknown action parsed")
	}
	if a, args, err := parseAction("  Grow right 200px "); err != nil || a.name != "grow" || strings.Join(args, " ") != "right 200px" {
		t.Fatalf("parsed %q %v %v", a.name, args, err)
	}
}

func TestRunActions(t *testing.T) {
	wm := newTestManager(t)
	s := wm.GetActiveScreen()
	do := func(line string) error {
		t.Helper()
		a, args, err := parseAction(line)
		if err != nil {
			t.Fatal(err)
		}
		err = wm.runAction(a, args)
		drainEvents(t, wm)
		return err
	}
	must := func(line string) {
		t.Helper()
		if err := do(line); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
	}
	a := newTestClient(t, wm)
	newTestClient(t, wm)
	drainEvents(t, wm)

	must("split right")
	left, right := wm.Clients[a].frame, wm.GetActiveFrame()
	w := right.g.W
	must("grow left 50")
	if right.g.W != w+50 {
		t.Fatalf("grow left 50: %d wide, was %d", right.g.W, w)
	}
	must("shrink left")
	if right.g.W != w-50 {
		t.Fatalf("shrink left: %d wide", right.g.W)
	}
	if err := do("grow right"); err == nil {
		t.Fatalf("grew past the screen's edge")
	}
	must("focus left")
	must("move-tab right")
	if len(left.clients) != 1 || len(right.clients) != 1 || wm.GetActiveFrame() != right {
		t.Fatalf("move-tab right: %d and %d tabs", len(left.clients), len(right.clients))
	}
	must("fullscreen on")
	if s.fullscreen.client == 0 {
		t.Fatalf("fullscreen on")
	}
	must("fullscreen on")
	if s.fullscreen.client == 0 {
		t.Fatalf("fullscreen on twice turned it off")
	}
	must("fullscreen off")
	must("workspace new")
	must("workspace 1")
	if s.GetActiveWorkspace() != s.Workspaces[0] {
		t.Fatalf("workspace 1")
	}
	must("panel ports")
	if !s.panel.shown || panelViews[s.panel.view] != "Ports" {
		t.Fatalf("panel ports")
	}
	must("panel hide")
	if err := do("workspace 9"); err == nil || !strings.Contains(err.Error(), "no workspace") {
		t.Fatalf("workspace 9: %v", err)
	}
	if err := do("split sideways"); err == nil {
		t.Fatalf("split sideways")
	}

	// a project, by its name
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "demo", ".git"), 0o755)
	os.WriteFile(filepath.Join(root, "demo", ".fion"), []byte("split down\n"), 0o644)
	t.Setenv("FION_PROJECTS", root)
	must("project demo")
	if ws := s.GetActiveWorkspace(); ws.name != "demo" || ws.Root.leaf {
		t.Fatalf("project demo: workspace %q", ws.name)
	}
	if err := do("project nothere"); err == nil {
		t.Fatalf("an unknown project opened")
	}
}
