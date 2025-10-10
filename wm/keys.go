package wm

import (
	"github.com/BurntSushi/xgb/xproto"
)

// -------------------------- Keys & Modifiers --------------------------

const Key_1 = xproto.Keycode(26)
const Key_2 = xproto.Keycode(27)
const Key_3 = xproto.Keycode(28)
const Key_4 = xproto.Keycode(29)
const Key_5 = xproto.Keycode(30)
const Key_6 = xproto.Keycode(31)
const Key_7 = xproto.Keycode(32)
const Key_8 = xproto.Keycode(33)
const Key_9 = xproto.Keycode(34)
const Key_0 = xproto.Keycode(35)

const Key_Q = 0x51
const Key_C = xproto.Keycode(16)
const Key_D = xproto.Keycode(10)
const Key_H = xproto.Keycode(12)
const Key_V = xproto.Keycode(17)
const Key_W = xproto.Keycode(21)

const Key_Space = xproto.Keycode(57)

const Key_F1 = xproto.Keycode(130)
const Key_F2 = xproto.Keycode(128)
const Key_F9 = xproto.Keycode(109)

const Key_BackQuote = xproto.Keycode(58)

const Key_LeftArrow = xproto.Keycode(131)
const Key_RightArrow = xproto.Keycode(132)
const Key_UpArrow = xproto.Keycode(134)
const Key_DownArrow = xproto.Keycode(133)

const KeyCMD = xproto.Keycode(63)

func (wm *Manager) grabKey(win xproto.Window, mods uint16, key xproto.Keycode) error {
	locks := []uint16{0, xproto.ModMaskLock, wm.NumLock, xproto.ModMaskLock | wm.NumLock}
	for _, m := range locks {
		if err := xproto.GrabKeyChecked(wm.Conn(), true, win, mods|m, key, xproto.GrabModeAsync, xproto.GrabModeAsync).Check(); err != nil {
			return err
		}
	}
	return nil
}
