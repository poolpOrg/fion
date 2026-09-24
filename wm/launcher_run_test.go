package wm

import (
	"os/exec"
	"strings"
	"testing"
)

func TestNeedsTerminal(t *testing.T) {
	l := newLauncher(launchItems(
		[]desktopApp{{name: "Firefox", exec: "firefox"}, {name: "Top", exec: "top", terminal: true}},
		[]string{"btop", "xeyes", "ls"}, nil))
	windowed := func(p string) bool { return p == "xeyes" }

	for line, want := range map[string]bool{
		"btop":                    true,
		"ls -l | sort":            true,
		"xeyes":                   false,
		"/usr/bin/xeyes -g 10x10": false,
		// applications say so themselves, whatever their programs link
		"firefox":                   false,
		"firefox https://fion.test": false,
		// and are already wrapped when they need a terminal
		"xterm -e top": false,
	} {
		if got := l.needsTerminal(line, windowed); got != want {
			t.Errorf("needsTerminal(%q) = %v, want %v", line, got, want)
		}
	}
}

// TestInTerminal runs the wrapper's script, without the terminal.
func TestInTerminal(t *testing.T) {
	line := `printf '%s|' "a b" c; exit 3`
	args := inTerminal(line)
	if args[0] != "xterm" || args[len(args)-1] != line {
		t.Fatalf("arguments %q", args)
	}

	// what follows xterm's -e
	var cmd []string
	for i, a := range args {
		if a == "-e" {
			cmd = args[i+1:]
		}
	}
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Stdin = strings.NewReader("\n")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("script: %v: %s", err, out)
	}
	if got := string(out); !strings.HasPrefix(got, "a b|c|") || !strings.Contains(got, "exited with 3") {
		t.Fatalf("script printed %q", got)
	}
}
