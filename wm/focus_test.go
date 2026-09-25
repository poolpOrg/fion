package wm

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestKeyboardFocus(t *testing.T) {
	wm := newTestManager(t)
	mod := wm.KeyboardManager.Mod
	left := newTestClient(t, wm)
	drainEvents(t, wm)
	checkFocus(t, wm, left)

	// the new frame is empty: the keys go nowhere
	press(t, wm, XK_Right, mod|xproto.ModMaskShift)
	checkFocus(t, wm, wm.noFocus)
	right := newTestClient(t, wm)
	drainEvents(t, wm)
	checkFocus(t, wm, right)

	press(t, wm, XK_Left, mod)
	checkFocus(t, wm, left)
	press(t, wm, XK_Right, mod)
	checkFocus(t, wm, right)

	// a click in the other frame's window focuses it
	wm.clientClicked(xproto.ButtonPressEvent{Event: left, Root: wm.Screens[0].Info().Root})
	drainEvents(t, wm)
	checkFocus(t, wm, left)
	if wm.GetActiveFrame() != wm.Clients[left].frame {
		t.Fatalf("the frame clicked isn't the active one")
	}
}
