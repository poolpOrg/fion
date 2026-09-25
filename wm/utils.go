package wm

import (
	"strings"
	"unicode/utf8"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// titleAtoms are _NET_WM_NAME and UTF8_STRING, by connection, interned
// once: titles are read every second.
var titleAtoms = map[*xgb.Conn][2]xproto.Atom{}

func netWMName(c *xgb.Conn) (name, utf8String xproto.Atom) {
	if a, ok := titleAtoms[c]; ok {
		return a[0], a[1]
	}
	intern := func(s string) xproto.Atom {
		r, err := xproto.InternAtom(c, false, uint16(len(s)), s).Reply()
		if err != nil {
			return xproto.AtomNone
		}
		return r.Atom
	}
	a := [2]xproto.Atom{intern("_NET_WM_NAME"), intern("UTF8_STRING")}
	titleAtoms[c] = a
	return a[0], a[1]
}

// getWindowName returns a window's title, in UTF-8: its _NET_WM_NAME, or
// its WM_NAME, in Latin-1 when of type STRING.
func getWindowName(c *xgb.Conn, w xproto.Window) string {
	const all = ^uint32(0) // request "all" bytes
	name, utf8String := netWMName(c)
	if r, err := xproto.GetProperty(c, false, w, name, utf8String, 0, all).Reply(); err == nil && r.ValueLen > 0 {
		return strings.ToValidUTF8(string(r.Value[:r.ValueLen]), "?")
	}
	r, err := xproto.GetProperty(c, false, w, xproto.AtomWmName, xproto.GetPropertyTypeAny, 0, all).Reply()
	if err != nil || r == nil || r.ValueLen == 0 {
		return ""
	}
	v := r.Value[:r.ValueLen]
	if r.Type == xproto.AtomString {
		return fromLatin1(v)
	}
	return strings.ToValidUTF8(string(v), "?")
}

func fromLatin1(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		sb.WriteRune(rune(c))
	}
	return sb.String()
}

// latin1 turns UTF-8 into Latin-1, for the fonts that have no more, the
// characters they lack as ?.
func latin1(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xff || r == utf8.RuneError {
			r = '?'
		}
		b = append(b, byte(r))
	}
	return string(b)
}

// char2b turns UTF-8 into the characters of a 16 bits font, those beyond
// its plane as ?.
func char2b(s string) []xproto.Char2b {
	out := make([]xproto.Char2b, 0, len(s))
	for _, r := range s {
		if r > 0xffff || r == utf8.RuneError {
			r = '?'
		}
		out = append(out, xproto.Char2b{Byte1: byte(r >> 8), Byte2: byte(r)})
	}
	return out
}

// truncateRunes cuts s to n characters.
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	i := 0
	for j := range s {
		if i == n {
			return s[:j]
		}
		i++
	}
	return s
}

// imageText draws UTF-8 text, as 16 bits characters when the GC's font is
// a Unicode one, in Latin-1 otherwise.
func imageText(c *xgb.Conn, d xproto.Drawable, gc xproto.Gcontext, unicode bool, x, y int16, s string) {
	if unicode {
		cs := char2b(s)
		if len(cs) > 255 {
			cs = cs[:255]
		}
		xproto.ImageText16(c, byte(len(cs)), d, gc, x, y, cs)
		return
	}
	s = latin1(s)
	if len(s) > 255 {
		s = s[:255]
	}
	xproto.ImageText8(c, byte(len(s)), d, gc, x, y, s)
}
