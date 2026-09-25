package wm

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

func TestMatch(t *testing.T) {
	for _, tc := range []struct {
		query, name string
		want        matchClass
	}{
		{"fire", "firefox", prefixMatch},
		{"FIRE", "firefox", prefixMatch},
		{"fox", "firefox", substringMatch},
		{"ffx", "firefox", fuzzyMatch},
		{"xff", "firefox", noMatch},
		{"", "anything", prefixMatch},
	} {
		if got, _ := match(tc.query, tc.name); got != tc.want {
			t.Errorf("match(%q, %q) = %d, want %d", tc.query, tc.name, got, tc.want)
		}
	}

	// tighter and shorter is better within a class
	_, tight := match("ffx", "ffxtool")
	_, loose := match("ffx", "firefox")
	if tight <= loose {
		t.Errorf("tight fuzzy match scored %d, not above loose %d", tight, loose)
	}
}

func labels(l *launcher) []string {
	var s []string
	for _, i := range l.matches {
		s = append(s, l.items[i].label)
	}
	return s
}

func TestLauncherRanking(t *testing.T) {
	items := launchItems(
		[]desktopApp{{name: "Firefox", exec: "firefox"}, {name: "Files", exec: "nautilus"}},
		nil,
		[]string{"firefox", "file", "fdisk", "xterm"},
		map[string]int{"fdisk": 3, "xterm -e top": 1},
	)

	l := newLauncher(items)
	l.setQuery("f")
	got := labels(l)
	// prefix matches, those run before first, then shorter first
	want := []string{"fdisk", "file", "Files", "Firefox"}
	if !slices.Equal(got[:4], want) {
		t.Fatalf("ranking for %q: %v, want %v first", "f", got, want)
	}
	if slices.Contains(got, "firefox") {
		t.Fatalf("firefox listed twice, as a command and as Firefox: %v", got)
	}

	// matched on the program an application runs
	l.setQuery("nauti")
	if got := labels(l); !slices.Equal(got, []string{"Files"}) {
		t.Fatalf("ranking for %q: %v, want [Files]", "nauti", got)
	}

	// without a query, what was run before, most used first
	l.setQuery("")
	if got := labels(l); !slices.Equal(got, []string{"fdisk", "xterm -e top"}) {
		t.Fatalf("empty query lists %v, want the history", got)
	}
}

func TestLauncherLine(t *testing.T) {
	items := launchItems(nil, nil, []string{"xterm", "xclock"}, map[string]int{"xterm -e top": 1})
	l := newLauncher(items)

	// the selection, which the history puts first
	l.setQuery("xte")
	if got := l.line(); got != "xterm -e top" {
		t.Fatalf("line for %q = %q, want the selection from the history", "xte", got)
	}

	// arguments: what was typed
	l.setQuery("xterm -e htop")
	if got := l.line(); got != "xterm -e htop" {
		t.Fatalf("line with arguments = %q, want it as typed", got)
	}
	// unless the selection was moved to
	l.setQuery("xterm -e")
	l.move(1)
	l.move(-1)
	if got := l.line(); got != "xterm -e top" {
		t.Fatalf("line with a moved selection = %q, want the selection", got)
	}

	// nothing matches: what was typed
	l.setQuery("  echo hello ")
	if got := l.line(); got != "echo hello" {
		t.Fatalf("line matching nothing = %q, want it as typed", got)
	}

	// Tab takes the selection, to add arguments
	l.setQuery("xcl")
	l.complete()
	if l.query != "xclock " {
		t.Fatalf("query after Tab = %q", l.query)
	}
	l.deleteWord()
	if l.query != "" {
		t.Fatalf("query after deleting a word = %q", l.query)
	}
}

func TestParseDesktopEntry(t *testing.T) {
	app, ok := parseDesktopEntry(strings.NewReader(`# comment
[Desktop Entry]
Type=Application
Name=Firefox
Name[fr]=Navigateur
Exec=firefox --new-window %u
Terminal=false

[Desktop Action private]
Name=Private window
Exec=firefox --private-window %u
`))
	if !ok || app.name != "Firefox" || app.exec != "firefox --new-window" || app.terminal {
		t.Fatalf("parsed %+v, %v", app, ok)
	}

	for _, entry := range []string{
		"[Desktop Entry]\nType=Application\nName=Hidden\nExec=hidden\nNoDisplay=true\n",
		"[Desktop Entry]\nType=Link\nName=Link\nURL=https://example.org\n",
		"[Desktop Entry]\nType=Application\nName=No exec\n",
	} {
		if app, ok := parseDesktopEntry(strings.NewReader(entry)); ok {
			t.Errorf("shown %+v from %q", app, entry)
		}
	}

	if got := stripFieldCodes(`gimp %F --scale 100%% %i`); got != "gimp --scale 100%" {
		t.Fatalf("stripFieldCodes = %q", got)
	}

	// terminal applications run in a terminal
	items := launchItems([]desktopApp{{name: "Top", exec: "top", terminal: true}}, nil, nil, nil)
	if items[0].command != "xterm -e top" {
		t.Fatalf("terminal application runs %q", items[0].command)
	}
}

func TestDesktopApps(t *testing.T) {
	user, system := t.TempDir(), t.TempDir()
	write := func(dir, name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entry := func(name string) string {
		return "[Desktop Entry]\nType=Application\nName=" + name + "\nExec=" + strings.ToLower(name) + "\n"
	}
	write(user, "editor.desktop", entry("Mine"))
	write(system, "editor.desktop", entry("Theirs"))
	write(system, "sub/other.desktop", entry("Other"))
	write(system, "notes.txt", "not an entry")

	var names []string
	for _, app := range desktopApps([]string{user, system}) {
		names = append(names, app.name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"Mine", "Other"}) {
		t.Fatalf("applications %v, want the user's entry to hide the system's", names)
	}
}

func TestPathCommands(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, f := range []struct {
		dir, name string
		mode      os.FileMode
	}{
		{a, "tool", 0o755}, {a, "data", 0o644}, {a, ".hidden", 0o755}, {b, "tool", 0o755}, {b, "other", 0o700},
	} {
		if err := os.WriteFile(filepath.Join(f.dir, f.name), nil, f.mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(b, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(a, "tool"), filepath.Join(b, "link")); err != nil {
		t.Fatal(err)
	}

	got := pathCommands(a + string(os.PathListSeparator) + b + string(os.PathListSeparator) + "/nonexistent")
	if want := []string{"link", "other", "tool"}; !slices.Equal(got, want) {
		t.Fatalf("commands %v, want %v", got, want)
	}
}

func TestHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fion", "history")
	if len(loadHistory(path)) != 0 {
		t.Fatalf("history found before any run")
	}
	for _, line := range []string{"xterm", "xterm -e top", "xterm"} {
		if err := appendHistory(path, line); err != nil {
			t.Fatal(err)
		}
	}
	uses := loadHistory(path)
	if uses["xterm"] != 2 || uses["xterm -e top"] != 1 || len(uses) != 2 {
		t.Fatalf("history %v", uses)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("history file mode %v, %v", info.Mode(), err)
	}
}

// TestLauncherRun types in the launcher, with XTEST, to run a command.
func TestLauncherRun(t *testing.T) {
	// a command that leaves its arguments in a file, alone in $PATH
	bin, state, mark := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "mark")
	script := "#!/bin/sh\necho \"ran $*\" >> " + mark + "\n"
	if err := os.WriteFile(filepath.Join(bin, "fiontestcmd"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/bin:/usr/bin")
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	// run it directly, not in a terminal
	launcherOpensWindows = func(string) bool { return true }
	t.Cleanup(func() { launcherOpensWindows = opensWindows })

	wm := newTestManager(t)
	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := xtest.Init(conn); err != nil {
		t.Skipf("XTEST: %v", err)
	}
	root := wm.GetActiveScreen().Info().Root
	setup := xproto.Setup(conn)
	mapping, err := xproto.GetKeyboardMapping(conn, setup.MinKeycode,
		byte(setup.MaxKeycode-setup.MinKeycode+1)).Reply()
	if err != nil {
		t.Fatal(err)
	}
	keycode := func(sym xproto.Keysym) byte {
		t.Helper()
		w := int(mapping.KeysymsPerKeycode)
		for i := 0; i*w < len(mapping.Keysyms); i++ {
			if mapping.Keysyms[i*w] == sym {
				return byte(int(setup.MinKeycode) + i)
			}
		}
		t.Fatalf("no keycode for keysym 0x%x", sym)
		return 0
	}
	superL := keycode(0xFFEB)
	press := func(syms ...xproto.Keysym) {
		t.Helper()
		for _, sym := range syms {
			kc := keycode(sym)
			for _, kind := range []byte{xproto.KeyPress, xproto.KeyRelease} {
				if err := xtest.FakeInputChecked(conn, kind, kc, 0, root, 0, 0, 0).Check(); err != nil {
					t.Fatal(err)
				}
			}
			drainEvents(t, wm)
		}
	}
	typeText := func(s string) {
		t.Helper()
		for _, r := range s {
			press(xproto.Keysym(r))
		}
	}
	openWithKeys := func() {
		t.Helper()
		for _, kind := range []byte{xproto.KeyPress} {
			xtest.FakeInputChecked(conn, kind, superL, 0, root, 0, 0, 0).Check()
		}
		press(XK_Return)
		xtest.FakeInputChecked(conn, xproto.KeyRelease, superL, 0, root, 0, 0, 0).Check()
		drainEvents(t, wm)
		if !wm.launcherOpen() {
			t.Fatalf("Super+Return didn't open the launcher")
		}
	}
	waitMark := func(want string) {
		t.Helper()
		for range 100 {
			if data, _ := os.ReadFile(mark); strings.Contains(string(data), want) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		data, _ := os.ReadFile(mark)
		t.Fatalf("command didn't run %q, mark has %q", want, data)
	}

	// Super+Return, a few letters, Return
	openWithKeys()
	typeText("fionte")
	if it, ok := wm.launcher.selection(); !ok || it.command != "fiontestcmd" {
		t.Fatalf("selection %+v after typing, want fiontestcmd", it)
	}
	attr, _ := xproto.GetWindowAttributes(wm.Conn(), wm.launcher.window).Reply()
	if attr.MapState != xproto.MapStateViewable {
		t.Fatalf("launcher window not shown")
	}
	press(XK_Return)
	if wm.launcherOpen() {
		t.Fatalf("launcher still open after Return")
	}
	waitMark("ran \n")

	// the history ranks it first with an empty query; Tab, then arguments
	openWithKeys()
	if it, ok := wm.launcher.selection(); !ok || it.command != "fiontestcmd" || it.uses != 1 {
		t.Fatalf("selection %+v with an empty query, want fiontestcmd from the history", it)
	}
	press(XK_Tab)
	typeText("hello")
	press(XK_Return)
	waitMark("ran hello")

	// Escape closes without running anything
	openWithKeys()
	typeText("fiontestcmd bye")
	press(XK_Escape)
	if wm.launcherOpen() {
		t.Fatalf("launcher still open after Escape")
	}
	time.Sleep(100 * time.Millisecond)
	if data, _ := os.ReadFile(mark); strings.Contains(string(data), "bye") {
		t.Fatalf("Escape ran the command")
	}
	if uses := loadHistory(filepath.Join(state, "fion", "history")); uses["fiontestcmd"] != 1 || uses["fiontestcmd hello"] != 1 {
		t.Fatalf("history %v", uses)
	}
}
