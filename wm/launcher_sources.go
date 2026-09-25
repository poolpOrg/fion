package wm

import (
	"bufio"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The launcher's sources: the executables in $PATH, the desktop
// applications, and the history of the command lines it ran.

// pathCommands returns the names of the executables in the directories of
// path, without duplicates, sorted.
func pathCommands(path string) []string {
	seen := map[string]bool{}
	var names []string
	for _, dir := range filepath.SplitList(path) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if seen[name] || strings.HasPrefix(name, ".") {
				continue
			}
			// follows symbolic links
			info, err := os.Stat(filepath.Join(dir, name))
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

type desktopApp struct {
	name     string
	exec     string
	terminal bool
}

// parseDesktopEntry reads a desktop entry file, and reports whether it is
// an application to show.
func parseDesktopEntry(r io.Reader) (desktopApp, bool) {
	var app desktopApp
	var typ string
	hidden, inEntry := false, false

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if !inEntry {
			continue
		}
		// localized keys, as Name[fr], are other keys
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "Type":
			typ = value
		case "Name":
			app.name = value
		case "Exec":
			app.exec = stripFieldCodes(value)
		case "Terminal":
			app.terminal = value == "true"
		case "NoDisplay", "Hidden":
			if value == "true" {
				hidden = true
			}
		}
	}
	return app, typ == "Application" && app.name != "" && app.exec != "" && !hidden
}

// stripFieldCodes removes from an Exec line the field codes, as %u or %F,
// that stand for files or URLs to open, which the launcher has none of.
func stripFieldCodes(exec string) string {
	var b strings.Builder
	for i := 0; i < len(exec); i++ {
		if exec[i] == '%' && i+1 < len(exec) {
			i++
			if exec[i] == '%' {
				b.WriteByte('%')
			}
			continue
		}
		b.WriteByte(exec[i])
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// applicationDirs returns the directories holding desktop entries, by
// decreasing precedence, following the XDG base directory specification.
func applicationDirs() []string {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dataHome = filepath.Join(home, ".local", "share")
		}
	}
	dataDirs := os.Getenv("XDG_DATA_DIRS")
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}

	var dirs []string
	for _, d := range append([]string{dataHome}, filepath.SplitList(dataDirs)...) {
		if d != "" {
			dirs = append(dirs, filepath.Join(d, "applications"))
		}
	}
	return dirs
}

// desktopApps returns the applications described in dirs. An entry in a
// directory hides the one with the same path in the directories after it.
func desktopApps(dirs []string) []desktopApp {
	seen := map[string]bool{}
	var apps []desktopApp
	for _, dir := range dirs {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".desktop") {
				return nil
			}
			id, _ := filepath.Rel(dir, path)
			if seen[id] {
				return nil
			}
			seen[id] = true
			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()
			if app, ok := parseDesktopEntry(f); ok {
				apps = append(apps, app)
			}
			return nil
		})
	}
	return apps
}

// historyPath is where the launcher keeps the command lines it ran.
func historyPath() string {
	return filepath.Join(stateDir(), "history")
}

// how many of the last command lines run are counted
const historySize = 1000

// loadHistory counts the uses of the last command lines run.
func loadHistory(path string) map[string]int {
	uses := map[string]int{}
	data, err := os.ReadFile(path)
	if err != nil {
		return uses
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > historySize {
		lines = lines[len(lines)-historySize:]
	}
	for _, line := range lines {
		if line != "" {
			uses[line]++
		}
	}
	return uses
}

// appendHistory records a command line run.
func appendHistory(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// launchItems merges the sources: an application, a project or a command
// line run before is shown once, the uses of the history added to it.
func launchItems(apps []desktopApp, projects []launchItem, commands []string, uses map[string]int) []launchItem {
	var items []launchItem
	byCommand := map[string]int{}
	add := func(it launchItem) {
		if _, ok := byCommand[it.command]; ok {
			return
		}
		byCommand[it.command] = len(items)
		items = append(items, it)
	}

	for _, app := range apps {
		command := app.exec
		if app.terminal {
			command = "xterm -e " + command
		}
		add(launchItem{label: app.name, command: command, kind: kindApp})
	}
	for _, p := range projects {
		add(p)
	}
	for _, c := range commands {
		add(launchItem{label: c, command: c, kind: kindCommand})
	}

	lines := make([]string, 0, len(uses))
	for line := range uses {
		lines = append(lines, line)
	}
	sort.Strings(lines)
	for _, line := range lines {
		if i, ok := byCommand[line]; ok {
			items[i].uses += uses[line]
			continue
		}
		add(launchItem{label: line, command: line, kind: kindHistory, uses: uses[line]})
	}
	return items
}
