package wm

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"
)

func TestControlRequests(t *testing.T) {
	reqs, err := controlRequests([]string{"msg", "-l", "error", "-t", "5", "build", "failed"}, nil)
	if err != nil || len(reqs) != 1 || reqs[0].Cmd != "msg" || reqs[0].Level != "error" || reqs[0].Timeout != 5 || reqs[0].Text != "build failed" {
		t.Fatalf("msg: %+v, %v", reqs, err)
	}
	reqs, err = controlRequests([]string{"msg", "-l", "ok"}, strings.NewReader("one\n\ntwo\n"))
	if err != nil || len(reqs) != 2 || reqs[1].Text != "two" || reqs[1].Level != "ok" {
		t.Fatalf("msg from stdin: %+v, %v", reqs, err)
	}
	reqs, err = controlRequests([]string{"ctl", "workspace", "next"}, nil)
	if err != nil || reqs[0].Cmd != "workspace" || reqs[0].Args[0] != "next" {
		t.Fatalf("ctl: %+v, %v", reqs, err)
	}
	for _, bad := range [][]string{{"msg", "-l", "loud", "x"}, {"msg", "-t", "soon", "x"}, {"ctl"}, {"msg", "-x", "y"}} {
		if _, err := controlRequests(bad, strings.NewReader("")); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	if ok, _ := Control([]string{"other"}, nil, nil); ok {
		t.Fatalf("Control took another command")
	}
}

func TestControlSocket(t *testing.T) {
	wm := newTestManager(t)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	// an event loop, for the requests
	wm.later = make(chan func(), 16)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case f := <-wm.later:
				f()
			case <-stop:
				return
			}
		}
	}()
	l, err := wm.listenControl(socketPath(os.Getenv("DISPLAY")))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if info, err := os.Stat(socketPath(os.Getenv("DISPLAY"))); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket %v, %v", info, err)
	}
	// a second fion doesn't take it over
	if _, err := wm.listenControl(socketPath(os.Getenv("DISPLAY"))); err == nil {
		t.Fatalf("the socket in use was replaced")
	}

	if _, err := Control([]string{"msg", "-l", "warn", "tests", "are", "slow"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	done := make(chan []*notification)
	wm.later <- func() { done <- append([]*notification(nil), wm.notes.shown...) }
	shown := <-done
	if len(shown) != 1 || shown[0].level != levelWarn || shown[0].text != "tests are slow" {
		t.Fatalf("shown %+v", shown)
	}
	if _, err := Control([]string{"ctl", "workspace", "new"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Control([]string{"ctl", "workspace", "9"}, nil, nil); err == nil || !strings.Contains(err.Error(), "no workspace") {
		t.Fatalf("workspace 9: %v", err)
	}
	wm.later <- func() { done <- nil }
	<-done
	if n := len(wm.GetActiveScreen().Workspaces); n != 2 {
		t.Fatalf("%d workspaces after ctl workspace new", n)
	}
}

func TestNotificationLine(t *testing.T) {
	wm := newTestManager(t)
	for i := range 6 {
		wm.notify(levelInfo, strings.Repeat("x", i+1), 0)
	}
	wm.notify(levelError, "  build\nfailed  ", 50*time.Millisecond)
	nl := wm.notes
	if len(nl.shown) != maxNotes || nl.shown[maxNotes-1].text != "build failed" || len(nl.history) != 7 {
		t.Fatalf("%d shown, %d kept, last %q", len(nl.shown), len(nl.history), nl.shown[len(nl.shown)-1].text)
	}
	drainEvents(t, wm)
	g, err := xproto.GetGeometry(wm.Conn(), xproto.Drawable(nl.window)).Reply()
	if err != nil {
		t.Fatal(err)
	}
	sg := wm.GetActiveScreen().Geometry()
	if int(g.Y)+int(g.Height) != int(sg.H)-infoBarOuterH() || g.Width != sg.W {
		t.Fatalf("notification line at %+v, not on the bar", g)
	}
	time.Sleep(60 * time.Millisecond)
	wm.expireNotifications()
	if len(nl.shown) != maxNotes-1 {
		t.Fatalf("the error didn't time out: %d shown", len(nl.shown))
	}
	press(t, wm, XK_n, wm.KeyboardManager.Mod)
	if len(nl.shown) != 0 {
		t.Fatalf("Mod+n didn't dismiss the messages")
	}
	if attr, _ := xproto.GetWindowAttributes(wm.Conn(), nl.window).Reply(); attr.MapState == xproto.MapStateViewable {
		t.Fatalf("the notification line shows empty")
	}
}
