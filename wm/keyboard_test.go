package wm

import (
	"testing"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// TestCheckMapping changes the keyboard mapping behind fion's back, as a
// switch between keyboards does without a MappingNotify.
func TestCheckMapping(t *testing.T) {
	wm := newTestManager(t)
	km := wm.KeyboardManager
	if km.CheckMapping() {
		t.Fatalf("mapping reported changed before any change")
	}

	conn, err := xgb.NewConn()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// make the last keycode produce w
	setup := xproto.Setup(conn)
	kc := setup.MaxKeycode
	old, err := xproto.GetKeyboardMapping(conn, kc, 1).Reply()
	if err != nil {
		t.Fatal(err)
	}
	set := func(syms []xproto.Keysym) {
		t.Helper()
		err := xproto.ChangeKeyboardMappingChecked(conn, 1, kc, old.KeysymsPerKeycode, syms).Check()
		if err != nil {
			t.Fatal(err)
		}
	}
	w := make([]xproto.Keysym, old.KeysymsPerKeycode)
	w[0] = XK_w
	set(w)
	defer set(old.Keysyms)

	if !km.CheckMapping() {
		t.Fatalf("mapping change not noticed")
	}
	if km.CheckMapping() {
		t.Fatalf("mapping reported changed twice")
	}
}
