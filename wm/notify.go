package wm

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jezek/xgb/xproto"
)

// The notification line, stacked on the bar of the monitor with the
// focus, shows the messages fion msg posts, and fion's own, the newest at
// the bottom, colored by level, until they time out, a click on it or
// Mod+n dismissing them. The panel's Messages view keeps the last ones.

type level int

const (
	levelInfo level = iota
	levelOK
	levelWarn
	levelError
)

var levelNames = []string{"info", "ok", "warn", "error"}

func parseLevel(s string) (level, error) {
	if s == "" {
		return levelInfo, nil
	}
	for i, n := range levelNames {
		if s == n || (n == "error" && s == "err") || (n == "warn" && s == "warning") {
			return level(i), nil
		}
	}
	return 0, fmt.Errorf("level %q: info, ok, warn or error", s)
}

func (l level) color() uint32 {
	switch l {
	case levelOK:
		return draculaGreen
	case levelWarn:
		return draculaYellow
	case levelError:
		return colorAlert
	}
	return draculaCyan
}

// defaultTimeout is how long a message of a level stays, unless told.
func (l level) defaultTimeout() time.Duration {
	switch l {
	case levelWarn:
		return 15 * time.Second
	case levelError:
		return 30 * time.Second
	}
	return 8 * time.Second
}

type notification struct {
	level level
	text  string
	at    time.Time
	until time.Time
}

// the notification line: at most this many messages at once, and this
// many kept for the panel
const maxNotes, noteHistory = 4, 100

type noteLine struct {
	window  xproto.Window
	gc      xproto.Gcontext
	unicode bool
	screen  *Screen
	shown   []*notification
	history []*notification
}

// notify posts a message on the notification line.
func (wm *Manager) notify(l level, text string, timeout time.Duration) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return
	}
	if r := []rune(text); len(r) > 300 {
		text = string(r[:300]) + "..."
	}
	if timeout <= 0 {
		timeout = l.defaultTimeout()
	}
	now := time.Now()
	n := &notification{level: l, text: text, at: now, until: now.Add(timeout)}
	nl := wm.noteLine()
	nl.shown = append(nl.shown, n)
	if len(nl.shown) > maxNotes {
		nl.shown = nl.shown[len(nl.shown)-maxNotes:]
	}
	nl.history = append(nl.history, n)
	if len(nl.history) > noteHistory {
		nl.history = nl.history[len(nl.history)-noteHistory:]
	}
	log.Printf("message, %s: %s", levelNames[l], text)
	wm.drawNotifications()
}

func (wm *Manager) noteLine() *noteLine {
	if wm.notes == nil {
		wm.notes = &noteLine{}
	}
	return wm.notes
}

// expireNotifications drops the messages timed out, every second.
func (wm *Manager) expireNotifications() {
	nl := wm.notes
	if nl == nil || len(nl.shown) == 0 {
		return
	}
	now := time.Now()
	kept := nl.shown[:0]
	for _, n := range nl.shown {
		if now.Before(n.until) {
			kept = append(kept, n)
		}
	}
	if len(kept) != len(nl.shown) {
		nl.shown = kept
		wm.drawNotifications()
	}
}

// clearNotifications dismisses the messages shown.
func (wm *Manager) clearNotifications() {
	if nl := wm.notes; nl != nil && len(nl.shown) > 0 {
		nl.shown = nil
		wm.drawNotifications()
	}
}

func noteLineH() int { return textH() + 4 }

// drawNotifications shows the messages above the bar of the monitor with
// the focus, and the panel when it is shown, or hides the line.
func (wm *Manager) drawNotifications() {
	nl := wm.noteLine()
	conn := wm.Conn()
	if len(nl.shown) == 0 {
		if nl.window != 0 {
			xproto.UnmapWindow(conn, nl.window)
		}
		return
	}
	s := wm.GetActiveScreen()
	if nl.window == 0 {
		w, err := xproto.NewWindowId(conn)
		if err != nil {
			return
		}
		xproto.CreateWindow(conn, s.Info().RootDepth, w, s.Info().Root, 0, 0, 1, 1, 0,
			xproto.WindowClassInputOutput, s.Info().RootVisual,
			xproto.CwBackPixel|xproto.CwOverrideRedirect|xproto.CwEventMask,
			[]uint32{colorBar, 1, xproto.EventMaskExposure | xproto.EventMaskButtonPress})
		gc, _ := xproto.NewGcontextId(conn)
		xproto.CreateGC(conn, gc, xproto.Drawable(w), xproto.GcForeground|xproto.GcBackground,
			[]uint32{colorText, colorBar})
		fid, ok := openFont(conn, font.unicode)
		nl.unicode = ok
		if !ok {
			fid, ok = openFont(conn, font.plain)
		}
		if ok {
			xproto.ChangeGC(conn, gc, xproto.GcFont, []uint32{uint32(fid)})
			xproto.CloseFont(conn, fid)
		}
		nl.window, nl.gc = w, gc
	}
	nl.screen = s
	g := s.Geometry()
	h := len(nl.shown)*noteLineH() + 1
	bottom := int(g.Y) + int(g.H) - infoBarOuterH()
	if s.panel != nil && s.panel.shown {
		bottom -= s.panel.h
	}
	xproto.ConfigureWindow(conn, nl.window,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight|xproto.ConfigWindowStackMode,
		[]uint32{uint32(g.X), uint32(bottom - h), uint32(g.W), uint32(h), xproto.StackModeAbove})
	xproto.MapWindow(conn, nl.window)
	nl.draw()
}

func (nl *noteLine) draw() {
	if nl.screen == nil || len(nl.shown) == 0 {
		return
	}
	conn, d := nl.screen.Conn(), xproto.Drawable(nl.window)
	w := int(nl.screen.Geometry().W)
	xproto.ClearArea(conn, false, nl.window, 0, 0, 0, 0)
	fill := func(x, y, w, h int, c uint32) {
		xproto.ChangeGC(conn, nl.gc, xproto.GcForeground, []uint32{c})
		xproto.PolyFillRectangle(conn, d, nl.gc, []xproto.Rectangle{{X: int16(x), Y: int16(y), Width: uint16(w), Height: uint16(h)}})
	}
	text := func(x, y int, s string, fg, bg uint32) {
		xproto.ChangeGC(conn, nl.gc, xproto.GcForeground|xproto.GcBackground, []uint32{fg, bg})
		imageText(conn, d, nl.gc, nl.unicode, int16(x), int16(y), s)
	}
	// a line on top, in the color of the most severe
	worst := levelInfo
	for _, n := range nl.shown {
		worst = max(worst, n.level)
	}
	fill(0, 0, w, 1, worst.color())
	chars := w/charW() - 2
	for i, n := range nl.shown {
		y := 1 + i*noteLineH()
		base := y + baseline(noteLineH())
		tag := fmt.Sprintf(" %-5s ", levelNames[n.level])
		fill(charW(), y+1, len(tag)*charW(), noteLineH()-2, n.level.color())
		text(charW(), base, tag, colorAccentText, n.level.color())
		stamp := n.at.Format("15:04:05")
		x := (len(tag) + 2) * charW()
		text(x, base, stamp, colorDim, colorBar)
		x += (len(stamp) + 2) * charW()
		text(x, base, truncateRunes(n.text, chars-len(tag)-len(stamp)-4), colorText, colorBar)
	}
}
