package wm

import (
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"
)

func TestFFmpegArgs(t *testing.T) {
	args := strings.Join(ffmpegArgs("127.0.0.1:3", rect{10, 20, 641, 479}, "/tmp/v.mp4"), " ")
	for _, want := range []string{"-f x11grab", "-video_size 640x478", "-i 127.0.0.1:3+10,20", "/tmp/v.mp4"} {
		if !strings.Contains(args, want) {
			t.Errorf("ffmpeg arguments %q lack %q", args, want)
		}
	}
}

func TestCapturePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_PICTURES_DIR", dir)
	at := time.Date(2026, 9, 25, 13, 2, 51, 0, time.UTC)
	if got := capturePath(at, ".png"); got != filepath.Join(dir, "fion-2026-09-25-130251.png") {
		t.Fatalf("capturePath = %q", got)
	}
}

// capturedPNGs returns the PNGs in dir.
func capturedPNGs(t *testing.T, dir string) []string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "fion-*.png"))
	return files
}

func TestScreenshots(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_PICTURES_DIR", dir)
	wm := newTestManager(t)
	mod, shift := wm.KeyboardManager.Mod, uint16(xproto.ModMaskShift)
	newTestClient(t, wm)
	press(t, wm, XK_Right, mod|shift) // an empty frame on the right, active

	// cancelled
	press(t, wm, XK_Print, 0)
	press(t, wm, XK_x, 0)
	if wm.mode != nil || len(capturedPNGs(t, dir)) != 0 {
		t.Fatalf("a cancelled capture left a mode or a file")
	}

	shot := func(target xproto.Keysym) (w, h int, path string) {
		t.Helper()
		before := capturedPNGs(t, dir)
		press(t, wm, XK_Print, 0)
		press(t, wm, XK_s, 0)
		press(t, wm, target, 0)
		after := capturedPNGs(t, dir)
		if len(after) != len(before)+1 {
			t.Fatalf("no screenshot saved in %s", dir)
		}
		path = after[len(after)-1]
		for _, b := range before {
			if b == path {
				path = after[0]
			}
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		// a second apart, for the next name
		time.Sleep(1100 * time.Millisecond)
		return img.Bounds().Dx(), img.Bounds().Dy(), path
	}

	// the frame: the empty right one, black below its title bar
	right := wm.GetActiveFrame()
	w, h, path := shot(XK_f)
	if w != int(right.g.W) || h != int(right.g.H) {
		t.Fatalf("frame screenshot %dx%d, want %dx%d", w, h, right.g.W, right.g.H)
	}
	f, _ := os.Open(path)
	img, _ := png.Decode(f)
	f.Close()
	if r, g, b, _ := img.At(5, titleH()+5).RGBA(); r|g|b != 0 {
		t.Fatalf("empty frame screenshot isn't black below its title bar")
	}

	// the workspace, the whole screen
	sg := wm.GetActiveScreen().Geometry()
	if w, h, _ := shot(XK_w); w != int(sg.W) || h != int(sg.H) {
		t.Fatalf("workspace screenshot %dx%d, want %dx%d", w, h, sg.W, sg.H)
	}

	// the tab, in the left frame
	press(t, wm, XK_Left, mod)
	left := wm.GetActiveFrame()
	if w, h, _ := shot(XK_t); w != int(left.g.W) || h != int(left.g.H)-titleH() {
		t.Fatalf("tab screenshot %dx%d, want %dx%d", w, h, left.g.W, int(left.g.H)-titleH())
	}
}

func TestRecording(t *testing.T) {
	out, err := exec.Command("ffmpeg", "-hide_banner", "-devices").Output()
	if err != nil || !strings.Contains(string(out), "x11grab") {
		t.Skip("no ffmpeg with x11grab")
	}
	dir := t.TempDir()
	t.Setenv("XDG_PICTURES_DIR", dir)
	wm := newTestManager(t)

	press(t, wm, XK_Print, 0)
	press(t, wm, XK_v, 0)
	press(t, wm, XK_w, 0)
	if wm.recording == nil {
		t.Fatalf("not recording")
	}
	time.Sleep(1500 * time.Millisecond)
	if err := wm.recordingFailed(); err != nil {
		t.Fatalf("recording failed: %v", err)
	}
	press(t, wm, XK_Print, 0)
	if wm.recording != nil {
		t.Fatalf("still recording after Print")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "fion-*.mp4"))
	if len(files) != 1 {
		t.Fatalf("videos %v", files)
	}
	if info, err := os.Stat(files[0]); err != nil || info.Size() < 1000 {
		t.Fatalf("video %v, %v", info, err)
	}
}
