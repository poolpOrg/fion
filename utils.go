package main

import (
	"math/rand"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

func (wm *WM) randomColor() uint32 {
	r := uint8(rand.Intn(256)) // Random value between 0-255
	g := uint8(rand.Intn(256))
	b := uint8(rand.Intn(256))
	return wm.allocColorRGB8(r, g, b)
}

func (wm *WM) allocColorRGB8(r8, g8, b8 uint8) uint32 {
	r := uint16(r8) * 0x101
	g := uint16(g8) * 0x101
	b := uint16(b8) * 0x101
	cm := wm.Scr.DefaultColormap
	rep, err := xproto.AllocColor(wm.X, cm, r, g, b).Reply()
	if err != nil {
		return 0
	}
	return rep.Pixel
}

func getWindowName(c *xgb.Conn, w xproto.Window) string {
	const all = ^uint32(0) // request "all" bytes
	r, err := xproto.GetProperty(c, false, w,
		xproto.AtomWmName, xproto.AtomString, 0, all).Reply()
	if err != nil || r == nil || r.ValueLen == 0 {
		return ""
	}
	return string(r.Value[:r.ValueLen])
}
