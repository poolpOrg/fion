package main

import (
	"log"
	"math/rand"
	"os"

	"github.com/BurntSushi/xgb/xproto"
)

var dbg = os.Getenv("WM_DEBUG") == "1"

func dprintf(format string, args ...any) {
	if dbg {
		log.Printf("[DBG] "+format, args...)
	}
}

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
