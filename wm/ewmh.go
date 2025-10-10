package wm

import (
	"encoding/binary"

	"github.com/BurntSushi/xgb/xproto"
)

func (wm *Manager) setProp32(win xproto.Window, prop, typ xproto.Atom, v uint32) {
	xproto.ChangeProperty(wm.Conn(), xproto.PropModeReplace, win, prop, typ, 32, 1, []uint8{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
}

func (wm *Manager) setPropStr(win xproto.Window, prop, typ xproto.Atom, s string) {
	data := []byte(s)
	xproto.ChangeProperty(wm.Conn(), xproto.PropModeReplace, win, prop, typ, 8, uint32(len(data)), data)
}

func (wm *Manager) setPropAtoms(win xproto.Window, prop xproto.Atom, atoms []xproto.Atom) {
	buf := make([]byte, 4*len(atoms))
	for i, a := range atoms {
		binary.LittleEndian.PutUint32(buf[i*4:(i+1)*4], uint32(a))
	}
	xproto.ChangeProperty(wm.Conn(), xproto.PropModeReplace, win, prop, xproto.AtomAtom, 32, uint32(len(atoms)), buf)
}
