package wm

import (
	"strings"
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestMergeResources(t *testing.T) {
	has := func(db, line string) bool {
		for _, l := range strings.Split(db, "\n") {
			if l == line {
				return true
			}
		}
		return false
	}

	// an empty database gets all of them
	merged := mergeResources("", xtermResources)
	for _, line := range xtermResources {
		if !has(merged, line) {
			t.Fatalf("%q missing from %q", line, merged)
		}
	}

	// the user's resources win, even set for every application
	user := "*background: white\nXTerm*vt100.color4: blue\n! *foreground: red\nXft.dpi: 96"
	merged = mergeResources(user, xtermResources)
	for _, line := range strings.Split(user, "\n") {
		if !has(merged, line) {
			t.Fatalf("user's %q lost in %q", line, merged)
		}
	}
	if has(merged, "XTerm*background: #282A36") || has(merged, "XTerm*color4: #BD93F9") {
		t.Fatalf("overrode the user's resources: %q", merged)
	}
	// commented out, so not set
	if !has(merged, "XTerm*foreground: #F8F8F2") {
		t.Fatalf("foreground missing though only commented out: %q", merged)
	}

	// merging again changes nothing
	if again := mergeResources(merged, xtermResources); again != merged {
		t.Fatalf("second merge changed the database:\n%q\n%q", merged, again)
	}
}

func TestInstallXtermTheme(t *testing.T) {
	wm := newTestManager(t)
	r, err := xproto.GetProperty(wm.Conn(), false, wm.Setup.Roots[0].Root, xproto.AtomResourceManager,
		xproto.AtomString, 0, ^uint32(0)).Reply()
	if err != nil {
		t.Fatal(err)
	}
	if db := string(r.Value[:r.ValueLen]); !strings.Contains(db, "XTerm*background: #282A36") {
		t.Fatalf("RESOURCE_MANAGER lacks the Dracula background: %q", db)
	}
}
