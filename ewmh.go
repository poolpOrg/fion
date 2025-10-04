package main

import (
	"encoding/binary"

	"github.com/BurntSushi/xgb/xproto"
)

func (wm *WM) initEWMH() error {
	X, A := wm.X, wm.Atoms
	w, err := xproto.NewWindowId(X)
	if err != nil {
		return err
	}
	xproto.CreateWindow(X, wm.Scr.RootDepth, w, wm.Root, 0, 0, 1, 1, 0,
		xproto.WindowClassInputOutput, wm.Scr.RootVisual, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange})
	wm.SupportingWin = w
	wm.setProp32(wm.Root, A.NET_SUPPORTING_WM_CHECK, xproto.AtomWindow, uint32(w))
	wm.setProp32(w, A.NET_SUPPORTING_WM_CHECK, xproto.AtomWindow, uint32(w))
	wm.setPropStr(w, A.NET_WM_NAME, A.UTF8_STRING, "fion")
	supported := []xproto.Atom{A.NET_SUPPORTED, A.NET_SUPPORTING_WM_CHECK, A.NET_CLIENT_LIST, A.NET_ACTIVE_WINDOW}
	wm.setPropAtoms(wm.Root, A.NET_SUPPORTED, supported)
	wm.updateClientList()
	return nil
}

func (wm *WM) setProp32(win xproto.Window, prop, typ xproto.Atom, v uint32) {
	xproto.ChangeProperty(wm.X, xproto.PropModeReplace, win, prop, typ, 32, 1, []uint8{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
}

func (wm *WM) setPropStr(win xproto.Window, prop, typ xproto.Atom, s string) {
	data := []byte(s)
	xproto.ChangeProperty(wm.X, xproto.PropModeReplace, win, prop, typ, 8, uint32(len(data)), data)
}

func (wm *WM) setPropAtoms(win xproto.Window, prop xproto.Atom, atoms []xproto.Atom) {
	buf := make([]byte, 4*len(atoms))
	for i, a := range atoms {
		binary.LittleEndian.PutUint32(buf[i*4:(i+1)*4], uint32(a))
	}
	xproto.ChangeProperty(wm.X, xproto.PropModeReplace, win, prop, xproto.AtomAtom, 32, uint32(len(atoms)), buf)
}
