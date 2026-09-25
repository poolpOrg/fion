package wm

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jezek/xgb/xproto"
)

// Print, without Mod, offers to capture the screen: a screenshot, saved as
// a PNG, or a video, recorded with ffmpeg until Print is pressed again, as
// an MP4 or a GIF, of the active tab, its frame, or the whole workspace.
// Captures are saved in $XDG_PICTURES_DIR, or ~/Pictures, or the home
// directory.

const XK_Print xproto.Keysym = 0xFF61

type rect struct{ x, y, w, h int }

// videoProblem tells why videos can't be recorded, and how to fix it, ""
// when they can: they need an ffmpeg that captures X, with x11grab.
func videoProblem(lookPath func(string) (string, error), devices func() (string, error), goos string) string {
	if _, err := lookPath("ffmpeg"); err != nil {
		return "videos need ffmpeg: " + installHint(goos)
	}
	if out, err := devices(); err != nil || !strings.Contains(out, "x11grab") {
		return "videos need an ffmpeg with x11grab: " + installHint(goos)
	}
	return ""
}

// installHint says how to install ffmpeg on a system.
func installHint(goos string) string {
	switch goos {
	case "openbsd":
		return "pkg_add ffmpeg"
	case "freebsd":
		return "pkg install ffmpeg"
	case "netbsd":
		return "pkgin install ffmpeg"
	case "linux":
		return "install your distribution's ffmpeg package"
	case "darwin":
		return "Homebrew's ffmpeg lacks it"
	}
	return "install ffmpeg"
}

// canRecord checks this system, each time, so that installing ffmpeg
// takes without restarting fion.
func canRecord() string {
	return videoProblem(exec.LookPath, func() (string, error) {
		out, err := exec.Command("ffmpeg", "-hide_banner", "-devices").Output()
		return string(out), err
	}, runtime.GOOS)
}

// captureTargets are the areas a capture may take, in root coordinates:
// the active tab, when there is one, its frame, the workspace.
func (wm *Manager) captureTargets() (tab rect, hasTab bool, frame, workspace rect) {
	s := wm.GetActiveScreen()
	g := s.Geometry()
	workspace = rect{int(g.X), int(g.Y), int(g.W), int(g.H)}
	if s.fullscreen.client != 0 {
		// the tab covers the screen
		return workspace, true, workspace, workspace
	}

	f := wm.GetActiveFrame()
	x, y := f.origin()
	frame = rect{int(x), int(y), int(f.g.W), int(f.g.H)}
	if f.floating() {
		// its border too
		frame = rect{int(f.g.X), int(f.g.Y), int(f.g.W) + 2, int(f.g.H) + 2}
	}
	if f.GetActiveClient() != 0 {
		c := f.clientGeometry()
		tab = rect{int(x) + int(c[0]), int(y) + int(c[1]), int(c[2]) + 2, int(c[3]) + 2}
		hasTab = true
	}
	return tab, hasTab, frame, workspace
}

// picturesDir is where captures are saved.
func picturesDir() string {
	if dir := os.Getenv("XDG_PICTURES_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	if info, err := os.Stat(filepath.Join(home, "Pictures")); err == nil && info.IsDir() {
		return filepath.Join(home, "Pictures")
	}
	return home
}

// capturePath names a capture taken at t.
func capturePath(t time.Time, ext string) string {
	return filepath.Join(picturesDir(), "fion-"+t.Format("2006-01-02-150405")+ext)
}

// screenshot reads r from the screen, as it shows, into an image.
func (wm *Manager) screenshot(r rect) (*image.RGBA, error) {
	s := wm.GetActiveScreen()
	img := image.NewRGBA(image.Rect(0, 0, r.w, r.h))
	lsb := wm.Setup.ImageByteOrder == xproto.ImageOrderLSBFirst
	// in bands, a reply being capped too
	band := max(1, (1<<22)/max(1, 4*r.w))
	for y0 := 0; y0 < r.h; y0 += band {
		n := min(band, r.h-y0)
		reply, err := xproto.GetImage(wm.Conn(), xproto.ImageFormatZPixmap, xproto.Drawable(s.Info().Root),
			int16(r.x), int16(r.y+y0), uint16(r.w), uint16(n), ^uint32(0)).Reply()
		if err != nil {
			return nil, err
		}
		if len(reply.Data) < 4*r.w*n {
			return nil, fmt.Errorf("not a 32 bits per pixel screen")
		}
		for i := 0; i < r.w*n; i++ {
			p := reply.Data[4*i : 4*i+4]
			c := color.RGBA{p[2], p[1], p[0], 255}
			if !lsb {
				c = color.RGBA{p[1], p[2], p[3], 255}
			}
			img.SetRGBA(i%r.w, y0+i/r.w, c)
		}
	}
	return img, nil
}

// saveScreenshot captures r into a PNG, and returns its path.
func (wm *Manager) saveScreenshot(r rect) (string, error) {
	img, err := wm.screenshot(r)
	if err != nil {
		return "", err
	}
	path := capturePath(time.Now(), ".png")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return "", err
	}
	return path, f.Close()
}

// ffmpegArgs are the arguments recording r of display to path: its size
// rounded down to even, as the encoders want.
func ffmpegArgs(display string, r rect, path string) []string {
	w, h := r.w&^1, r.h&^1
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "x11grab", "-framerate", "30", "-video_size", fmt.Sprintf("%dx%d", w, h),
		"-i", fmt.Sprintf("%s+%d,%d", display, r.x, r.y),
		"-pix_fmt", "yuv420p", path,
	}
}

type recording struct {
	cmd     *exec.Cmd
	path    string
	started time.Time
	done    chan error
	gif     bool // path is a temporary MP4, turned into a GIF once stopped
}

// gifArgs are the arguments turning the video in into the GIF out: 15
// frames a second, 1280 pixels wide at most, with a palette of its own.
func gifArgs(in, out string) []string {
	return []string{
		"-hide_banner", "-loglevel", "error", "-y", "-i", in,
		"-vf", "fps=15,scale='min(1280,iw)':-1:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse",
		out,
	}
}

// startRecording records r until stopRecording, for a GIF when gif.
func (wm *Manager) startRecording(r rect, gif bool) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("recording needs ffmpeg")
	}
	path := capturePath(time.Now(), ".mp4")
	if gif {
		f, err := os.CreateTemp("", "fion-*.mp4")
		if err != nil {
			return err
		}
		f.Close()
		path = f.Name()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("ffmpeg", ffmpegArgs(os.Getenv("DISPLAY"), r, path)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	rec := &recording{cmd: cmd, path: path, started: time.Now(), done: make(chan error, 1), gif: gif}
	go func() {
		err := cmd.Wait()
		if err != nil && stderr.Len() > 0 {
			err = fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
		}
		rec.done <- err
	}()
	wm.recording = rec
	return nil
}

// stopRecording has ffmpeg finish the file, and returns its path.
func (wm *Manager) stopRecording() (string, error) {
	rec := wm.recording
	wm.recording = nil
	// ffmpeg writes the file's index on an interrupt, and exits
	rec.cmd.Process.Signal(os.Interrupt)
	select {
	case err := <-rec.done:
		if _, statErr := os.Stat(rec.path); statErr != nil {
			return "", fmt.Errorf("recording failed: %v", err)
		}
		if rec.gif {
			wm.convertToGIF(rec.path, capturePath(rec.started, ".gif"))
			return "", nil
		}
		return rec.path, nil
	case <-time.After(5 * time.Second):
		rec.cmd.Process.Kill()
		return rec.path, fmt.Errorf("ffmpeg didn't stop, killed")
	}
}

// convertToGIF turns the video in into the GIF out, away from the event
// loop, removes in, and tells when done.
func (wm *Manager) convertToGIF(in, out string) {
	go func() {
		var stderr strings.Builder
		cmd := exec.Command("ffmpeg", gifArgs(in, out)...)
		cmd.Stderr = &stderr
		err := cmd.Run()
		os.Remove(in)
		done := func() {
			if err != nil {
				wm.alert("GIF: " + strings.TrimSpace(err.Error()+" "+stderr.String()))
				return
			}
			wm.notice("Saved " + out)
		}
		if wm.later == nil {
			done()
			return
		}
		wm.later <- done
	}()
}

// recordingFailed reports ffmpeg exiting on its own, and forgets the
// recording.
func (wm *Manager) recordingFailed() error {
	rec := wm.recording
	if rec == nil {
		return nil
	}
	select {
	case err := <-rec.done:
		wm.recording = nil
		if err == nil {
			err = fmt.Errorf("ffmpeg stopped")
		}
		return err
	default:
		return nil
	}
}

// printScreen offers the captures, or stops the recording.
func (wm *Manager) printScreen() error {
	if wm.recording != nil {
		path, err := wm.stopRecording()
		if err != nil {
			wm.notice("Video: " + err.Error())
			return err
		}
		if path == "" {
			wm.notice("Making the GIF...")
			return nil
		}
		wm.notice("Saved " + path)
		return nil
	}

	video, gif := false, false
	problem := canRecord()
	menu := "Capture: s a screenshot, v a video, g a GIF, any other key cancels"
	if problem != "" {
		menu = "Capture: s a screenshot, v a video or g a GIF (unavailable: " + problem + "), any other key cancels"
	}
	targets := func() string {
		what := "Screenshot"
		if gif {
			what = "GIF"
		} else if video {
			what = "Video"
		}
		if _, hasTab, _, _ := wm.captureTargets(); hasTab {
			return what + " of: t the tab, f the frame, w the workspace, any other key cancels"
		}
		return what + " of: f the frame, w the workspace, any other key cancels"
	}
	p, err := wm.showPrompt(menu, colorAccent)
	if err != nil {
		return err
	}
	chose := false
	wm.mode = &keyMode{key: func(sym xproto.Keysym) bool {
		if !chose {
			switch {
			case (sym == XK_v || sym == XK_g) && problem != "":
				// once the menu is gone, as it takes the prompt's place
				wm.after(0, func() { wm.alert("Can't record: " + problem) })
				return true
			case sym == XK_s, sym == XK_v, sym == XK_g:
				chose, video, gif = true, sym != XK_s, sym == XK_g
				p.setText(targets())
				return false
			}
			return true
		}
		tab, hasTab, frame, workspace := wm.captureTargets()
		var r rect
		switch {
		case sym == XK_t && hasTab:
			r = tab
		case sym == XK_f:
			r = frame
		case sym == XK_w:
			r = workspace
		default:
			return true
		}
		// once the prompt is gone and what it covered is drawn again
		wm.after(200*time.Millisecond, func() { wm.capture(r, video, gif) })
		return true
	}}
	return nil
}

// capture takes a screenshot of r, or starts recording it, for a GIF when
// gif.
func (wm *Manager) capture(r rect, video, gif bool) {
	if video {
		if err := wm.startRecording(r, gif); err != nil {
			wm.alert("Can't record: " + err.Error())
		}
		return
	}
	path, err := wm.saveScreenshot(r)
	if err != nil {
		wm.notice("Screenshot: " + err.Error())
		log.Printf("screenshot: %v", err)
		return
	}
	wm.notice("Saved " + path)
}

// after runs f on the event loop in d, or right away without one, as in
// tests.
func (wm *Manager) after(d time.Duration, f func()) {
	if wm.later == nil {
		f()
		return
	}
	time.AfterFunc(d, func() { wm.later <- f })
}

// notice shows a line at the top of the screen for a few seconds, without
// taking the keyboard.
func (wm *Manager) notice(text string) {
	wm.showNotice(text, colorAccent, 3*time.Second)
}

// alert is a notice about what went wrong, in red and for longer.
func (wm *Manager) alert(text string) {
	log.Print(text)
	wm.showNotice(text, colorAlert, 6*time.Second)
}

func (wm *Manager) showNotice(text string, color uint32, d time.Duration) {
	s := wm.GetActiveScreen()
	p, err := wm.promptWindow(s)
	if err != nil || p.shown {
		log.Print(text)
		return
	}
	p.text = text
	xproto.ChangeWindowAttributes(wm.Conn(), p.window, xproto.CwBorderPixel, []uint32{color})
	p.place()
	xproto.MapWindow(wm.Conn(), p.window)
	p.draw()
	p.noticeShown = true
	wm.after(d, func() {
		if p.noticeShown && !p.shown {
			p.noticeShown = false
			xproto.UnmapWindow(wm.Conn(), p.window)
		}
	})
}
