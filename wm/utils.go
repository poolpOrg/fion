package wm

import (
	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

func getWindowName(c *xgb.Conn, w xproto.Window) string {
	const all = ^uint32(0) // request "all" bytes
	r, err := xproto.GetProperty(c, false, w,
		xproto.AtomWmName, xproto.AtomString, 0, all).Reply()
	if err != nil || r == nil || r.ValueLen == 0 {
		return ""
	}
	return string(r.Value[:r.ValueLen])
}
