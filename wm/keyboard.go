package wm

import (
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

const (
	XK_Escape xproto.Keysym = 0xFF1B

	XK_F1  xproto.Keysym = 0xFFBE
	XK_F2  xproto.Keysym = 0xFFBF
	XK_F3  xproto.Keysym = 0xFFC0
	XK_F4  xproto.Keysym = 0xFFC1
	XK_F5  xproto.Keysym = 0xFFC2
	XK_F6  xproto.Keysym = 0xFFC3
	XK_F7  xproto.Keysym = 0xFFC4
	XK_F8  xproto.Keysym = 0xFFC5
	XK_F9  xproto.Keysym = 0xFFC6
	XK_F10 xproto.Keysym = 0xFFC7
	XK_F11 xproto.Keysym = 0xFFC8
	XK_F12 xproto.Keysym = 0xFFC9

	XK_Left  xproto.Keysym = 0xFF51
	XK_Up    xproto.Keysym = 0xFF52
	XK_Right xproto.Keysym = 0xFF53
	XK_Down  xproto.Keysym = 0xFF54

	XK_Space xproto.Keysym = 0x0020
	XK_Plus  xproto.Keysym = 0x002B // '+'
	XK_Minus xproto.Keysym = 0x002D // '-'

	XK_A xproto.Keysym = 0x0041
	XK_B xproto.Keysym = 0x0042
	XK_C xproto.Keysym = 0x0043
	XK_D xproto.Keysym = 0x0044
	XK_E xproto.Keysym = 0x0045
	XK_F xproto.Keysym = 0x0046
	XK_G xproto.Keysym = 0x0047
	XK_H xproto.Keysym = 0x0048
	XK_I xproto.Keysym = 0x0049
	XK_J xproto.Keysym = 0x004A
	XK_K xproto.Keysym = 0x004B
	XK_L xproto.Keysym = 0x004C
	XK_M xproto.Keysym = 0x004D
	XK_N xproto.Keysym = 0x004E
	XK_O xproto.Keysym = 0x004F
	XK_P xproto.Keysym = 0x0050
	XK_Q xproto.Keysym = 0x0051
	XK_R xproto.Keysym = 0x0052
	XK_S xproto.Keysym = 0x0053
	XK_T xproto.Keysym = 0x0054
	XK_U xproto.Keysym = 0x0055
	XK_V xproto.Keysym = 0x0056
	XK_W xproto.Keysym = 0x0057
	XK_X xproto.Keysym = 0x0058
	XK_Y xproto.Keysym = 0x0059
	XK_Z xproto.Keysym = 0x005A

	XK_a xproto.Keysym = 0x0061
	XK_b xproto.Keysym = 0x0062
	XK_c xproto.Keysym = 0x0063
	XK_d xproto.Keysym = 0x0064
	XK_e xproto.Keysym = 0x0065
	XK_f xproto.Keysym = 0x0066
	XK_g xproto.Keysym = 0x0067
	XK_h xproto.Keysym = 0x0068
	XK_i xproto.Keysym = 0x0069
	XK_j xproto.Keysym = 0x006A
	XK_k xproto.Keysym = 0x006B
	XK_l xproto.Keysym = 0x006C
	XK_m xproto.Keysym = 0x006D
	XK_n xproto.Keysym = 0x006E
	XK_o xproto.Keysym = 0x006F
	XK_p xproto.Keysym = 0x0070
	XK_q xproto.Keysym = 0x0071
	XK_r xproto.Keysym = 0x0072
	XK_s xproto.Keysym = 0x0073
	XK_t xproto.Keysym = 0x0074
	XK_u xproto.Keysym = 0x0075
	XK_v xproto.Keysym = 0x0076
	XK_w xproto.Keysym = 0x0077
	XK_x xproto.Keysym = 0x0078
	XK_y xproto.Keysym = 0x0079
	XK_z xproto.Keysym = 0x007A
)

type KeyboardManager struct {
	wm    *Manager
	Super uint16 // actual Mod* bit for Super
	Num   uint16 // NumLock mask
	Mode  uint16 // Mode_switch (AltGr) mask
}

func NewKeyboardManager(wm *Manager) *KeyboardManager {
	k := &KeyboardManager{wm: wm}
	k.Super = k.detectSuperMask()
	k.Num, k.Mode = k.detectModifierMasks()
	return k
}

func (k *KeyboardManager) Conn() *xgb.Conn {
	return k.wm.xConn
}

/* ---------------- core: portable grabs ---------------- */

// Grab keysym with baseMods on *all* producing keycodes,
// including Caps/Num permutations, and adding Shift/AltGr if needed.
func (k *KeyboardManager) GrabSym(win xproto.Window, sym xproto.Keysym, baseMods uint16) error {
	kcs := k.keycodesForSym(sym)
	if len(kcs) == 0 {
		return fmt.Errorf("keysym 0x%x not present", uint32(sym))
	}
	needShift, needMode := k.levelsFor(sym)
	m := baseMods
	if needShift {
		m |= xproto.ModMaskShift
	}
	if needMode {
		m |= k.Mode
	}

	var variants = []uint16{
		m,
		m | xproto.ModMaskLock, // Caps
		m | k.Num,              // Num
		m | xproto.ModMaskLock | k.Num,
	}

	for _, kc := range kcs {
		for _, mods := range variants {
			if err := xproto.GrabKeyChecked(
				k.Conn(), false, win, mods, kc,
				xproto.GrabModeAsync, xproto.GrabModeAsync,
			).Check(); err != nil {
				return err
			}
		}
	}
	return nil
}

// Convenience: printable rune (ASCII/Latin-1) → keysym → grab.
func (k *KeyboardManager) GrabRune(win xproto.Window, r rune, baseMods uint16) error {
	if r < 0 || r > 0xFF {
		return fmt.Errorf("unsupported rune %q", r)
	}
	return k.GrabSym(win, xproto.Keysym(uint32(byte(r))), baseMods)
}

// Convenience: a few named keys without hex at call-sites.
func (k *KeyboardManager) GrabNamed(win xproto.Window, name string, baseMods uint16) error {
	table := map[string]xproto.Keysym{
		"Return": 0xFF0D, "Enter": 0xFF0D,
		"Tab": 0xFF09, "BackSpace": 0xFF08, "Escape": 0xFF1B,
		"Space": 0x0020, "BackQuote": 0x0060,
		"Left": 0xFF51, "Up": 0xFF52, "Right": 0xFF53, "Down": 0xFF54,
		"F1": 0xFFBE, "F2": 0xFFBF, "F3": 0xFFC0, "F4": 0xFFC1,
		"F5": 0xFFC2, "F6": 0xFFC3, "F7": 0xFFC4, "F8": 0xFFC5,
		"F9": 0xFFC6, "F10": 0xFFC7, "F11": 0xFFC8, "F12": 0xFFC9,
	}
	sym, ok := table[name]
	if !ok {
		return fmt.Errorf("unknown key %q", name)
	}
	return k.GrabSym(win, sym, baseMods)
}

/* --------------- minimal mapping helpers --------------- */

// All keycodes that produce sym in any column.
func (k *KeyboardManager) keycodesForSym(sym xproto.Keysym) []xproto.Keycode {
	setup := xproto.Setup(k.Conn())
	min, max := int(setup.MinKeycode), int(setup.MaxKeycode)
	rep, err := xproto.GetKeyboardMapping(k.Conn(), xproto.Keycode(setup.MinKeycode), byte(max-min+1)).Reply()
	if err != nil || rep == nil || rep.KeysymsPerKeycode == 0 {
		return nil
	}
	w := int(rep.KeysymsPerKeycode)

	var out []xproto.Keycode
	for kc := min; kc <= max; kc++ {
		base := (kc - min) * w
		for col := 0; col < w; col++ {
			if rep.Keysyms[base+col] == sym {
				out = append(out, xproto.Keycode(kc))
				break
			}
		}
	}
	return out
}

// Does the symbol live in Shift / AltGr columns (so we must include those mods)?
func (k *KeyboardManager) levelsFor(sym xproto.Keysym) (needShift, needMode bool) {
	setup := xproto.Setup(k.Conn())
	min, max := int(setup.MinKeycode), int(setup.MaxKeycode)
	rep, err := xproto.GetKeyboardMapping(k.Conn(), xproto.Keycode(setup.MinKeycode), byte(max-min+1)).Reply()
	if err != nil || rep == nil || rep.KeysymsPerKeycode == 0 {
		return
	}
	w := int(rep.KeysymsPerKeycode)

	for kc := min; kc <= max; kc++ {
		base := (kc - min) * w
		for col := 0; col < w; col++ {
			if rep.Keysyms[base+col] != sym {
				continue
			}
			switch col {
			case 0:
				return false, false
			case 1:
				return true, false
			case 2:
				return false, true
			case 3:
				return true, true
			default:
				return (col%2 == 1), (col >= 2)
			}
		}
	}
	return
}

/* --------------- modifier detection (strict minimum) --------------- */

func (k *KeyboardManager) detectSuperMask() uint16 {
	const XK_Super_L, XK_Super_R = 0xFFE3, 0xFFE4
	mm, err := xproto.GetModifierMapping(k.Conn()).Reply()
	if err != nil || mm == nil {
		return xproto.ModMask4
	}

	setup := xproto.Setup(k.Conn())
	min, max := int(setup.MinKeycode), int(setup.MaxKeycode)
	km, err := xproto.GetKeyboardMapping(k.Conn(), xproto.Keycode(setup.MinKeycode), byte(max-min+1)).Reply()
	if err != nil || km == nil {
		return xproto.ModMask4
	}
	w := int(km.KeysymsPerKeycode)

	has := func(kc xproto.Keycode, sym xproto.Keysym) bool {
		base := (int(kc) - min) * w
		for c := 0; c < w; c++ {
			if km.Keysyms[base+c] == sym {
				return true
			}
		}
		return false
	}
	mods := []uint16{
		xproto.ModMaskShift, xproto.ModMaskLock, xproto.ModMaskControl,
		xproto.ModMask1, xproto.ModMask2, xproto.ModMask3, xproto.ModMask4, xproto.ModMask5,
	}
	kpm := int(mm.KeycodesPerModifier)
	for i, mask := range mods {
		for j := 0; j < kpm; j++ {
			kc := mm.Keycodes[i*kpm+j]
			if kc != 0 && (has(kc, xproto.Keysym(XK_Super_L)) || has(kc, xproto.Keysym(XK_Super_R))) {
				return mask
			}
		}
	}
	return xproto.ModMask4
}

func (k *KeyboardManager) detectModifierMasks() (num, mode uint16) {
	const XK_Num_Lock, XK_Mode_switch = 0xFF7F, 0xFF7E
	mm, err := xproto.GetModifierMapping(k.Conn()).Reply()
	if err != nil || mm == nil {
		return xproto.ModMask2, xproto.ModMask5
	}

	setup := xproto.Setup(k.Conn())
	min, max := int(setup.MinKeycode), int(setup.MaxKeycode)
	km, err := xproto.GetKeyboardMapping(k.Conn(), xproto.Keycode(setup.MinKeycode), byte(max-min+1)).Reply()
	if err != nil || km == nil {
		return xproto.ModMask2, xproto.ModMask5
	}
	w := int(km.KeysymsPerKeycode)

	has := func(kc xproto.Keycode, sym xproto.Keysym) bool {
		base := (int(kc) - min) * w
		for c := 0; c < w; c++ {
			if km.Keysyms[base+c] == sym {
				return true
			}
		}
		return false
	}
	mods := []uint16{
		xproto.ModMaskShift, xproto.ModMaskLock, xproto.ModMaskControl,
		xproto.ModMask1, xproto.ModMask2, xproto.ModMask3, xproto.ModMask4, xproto.ModMask5,
	}
	kpm := int(mm.KeycodesPerModifier)
	for i, mask := range mods {
		for j := 0; j < kpm; j++ {
			kc := mm.Keycodes[i*kpm+j]
			if kc == 0 {
				continue
			}
			if has(kc, xproto.Keysym(XK_Num_Lock)) {
				num = mask
			}
			if has(kc, xproto.Keysym(XK_Mode_switch)) {
				mode = mask
			}
		}
	}
	if num == 0 {
		num = xproto.ModMask2
	}
	if mode == 0 {
		mode = xproto.ModMask5
	}
	return
}

func (km *KeyboardManager) eventKeysym(kc xproto.Keycode, state uint16) xproto.Keysym {
	setup := xproto.Setup(km.Conn())
	min, max := int(setup.MinKeycode), int(setup.MaxKeycode)
	rep, err := xproto.GetKeyboardMapping(km.Conn(), xproto.Keycode(setup.MinKeycode), byte(max-min+1)).Reply()
	if err != nil || rep == nil || rep.KeysymsPerKeycode == 0 {
		return 0
	}
	w := int(rep.KeysymsPerKeycode)

	idx := int(kc) - min
	if idx < 0 || idx >= (max-min+1) {
		return 0
	}
	base := idx * w

	col := 0
	if (state & xproto.ModMaskShift) != 0 {
		col = 1
	}
	if (state & km.Mode) != 0 {
		col += 2
		if col >= w {
			col = w - 1
		}
	}

	return rep.Keysyms[base+col]
}
