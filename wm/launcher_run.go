package wm

import (
	"debug/elf"
	"debug/macho"
	"os/exec"
	"path/filepath"
	"strings"
)

// A program run from the launcher gets no terminal of its own: those that
// don't open windows, such as ls or btop, run in an xterm so that they can
// be seen.

// the libraries of programs that open windows
var windowingLibraries = []string{
	"libX11", "libxcb", "libgtk", "libgdk", "libQt", "libwayland-client", "libSDL",
	"AppKit.framework", "Cocoa.framework",
}

// launcherOpensWindows is how the launcher tells, replaced in tests.
var launcherOpensWindows = opensWindows

// opensWindows reports whether program, looked up in $PATH, is linked to a
// windowing library. Scripts, and programs that can't be read, count as
// not opening windows.
func opensWindows(program string) bool {
	path, err := exec.LookPath(program)
	if err != nil {
		return false
	}
	for _, lib := range importedLibraries(path) {
		for _, w := range windowingLibraries {
			if strings.Contains(lib, w) {
				return true
			}
		}
	}
	return false
}

// importedLibraries returns the libraries an ELF or Mach-O binary loads.
func importedLibraries(path string) []string {
	if f, err := elf.Open(path); err == nil {
		defer f.Close()
		libs, _ := f.ImportedLibraries()
		return libs
	}
	if f, err := macho.Open(path); err == nil {
		defer f.Close()
		libs, _ := f.ImportedLibraries()
		return libs
	}
	if f, err := macho.OpenFat(path); err == nil {
		defer f.Close()
		if len(f.Arches) > 0 {
			libs, _ := f.Arches[0].ImportedLibraries()
			return libs
		}
	}
	return nil
}

// program returns the program a command line runs.
func program(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return filepath.Base(fields[0])
}

// needsTerminal tells whether line runs a program that doesn't open
// windows. The desktop applications say so themselves, and their commands
// are already run in a terminal when needed.
func (l *launcher) needsTerminal(line string, opensWindows func(string) bool) bool {
	p := program(line)
	for _, it := range l.items {
		if it.kind == kindApp && (it.command == line || program(it.command) == p) {
			return false
		}
	}
	return !opensWindows(p)
}

// inTerminal returns the arguments running line in an xterm that stays
// open once it is done, until Return is pressed, so that its output can
// be read.
func inTerminal(line string) []string {
	script := `sh -c "$1"; status=$?; printf '\n[exited with %d, Return closes]' "$status"; read _`
	return []string{"xterm", "-T", line, "-e", "/bin/sh", "-c", script, "fion", line}
}
