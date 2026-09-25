package wm

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func TestFocusedDir(t *testing.T) {
	wm := newTestManager(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// a "terminal", its "shell" in dir
	cmd := exec.Command("sh", "-c", "cd "+dir+" && sleep 30 & wait")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	win := newTestWindow(t, wm, 100, 100, func(conn *xgb.Conn, w xproto.Window) {
		setWindowProp(conn, w, wm.Screens[0].atoms.NET_WM_PID, xproto.AtomCardinal, uint32(cmd.Process.Pid))
	})
	wm.manage(win, false)
	drainEvents(t, wm)

	var got string
	for range 50 {
		// the shell may not be in dir yet
		if got = wm.focusedDir(); got == dir {
			break
		}
		exec.Command("sleep", "0.05").Run()
	}
	if got != dir {
		home, _ := os.UserHomeDir()
		t.Fatalf("focused dir %q, want %q (home %q)", got, dir, home)
	}
}
