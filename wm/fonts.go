package wm

import (
	"log"
	"os"
	"strconv"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// fion draws its text with a core font of the fixed family, sized for the
// screen, or the one FION_FONT names, and sizes the bars, tabs and panels
// from its metrics.

type fontSpec struct {
	size    int
	plain   string
	bold    string // "" when the size has none: bold is then drawn twice
	unicode string // the same, for xterm
}

// the fixed sizes, the screen's height from which each is used
var fixedFonts = []struct {
	minScreenH int
	fontSpec
}{
	{0, fontSpec{13,
		"-misc-fixed-medium-r-semicondensed--13-120-75-75-c-60-iso8859-1",
		"-misc-fixed-bold-r-semicondensed--13-120-75-75-c-60-iso8859-1",
		"-misc-fixed-medium-r-semicondensed--13-120-75-75-c-60-iso10646-1"}},
	{1000, fontSpec{15,
		"-misc-fixed-medium-r-normal--15-140-75-75-c-90-iso8859-1",
		"-misc-fixed-bold-r-normal--15-140-75-75-c-90-iso8859-1",
		"-misc-fixed-medium-r-normal--15-140-75-75-c-90-iso10646-1"}},
	{1400, fontSpec{18,
		"-misc-fixed-medium-r-normal--18-120-100-100-c-90-iso8859-1",
		"-misc-fixed-bold-r-normal--18-120-100-100-c-90-iso8859-1",
		"-misc-fixed-medium-r-normal--18-120-100-100-c-90-iso10646-1"}},
	{1800, fontSpec{20,
		"-misc-fixed-medium-r-normal--20-200-75-75-c-100-iso8859-1",
		"",
		"-misc-fixed-medium-r-normal--20-200-75-75-c-100-iso10646-1"}},
}

// chooseFont picks the font for a screen screenH pixels high, or the one
// setting, FION_FONT's value, names: a size of the fixed family, as 18,
// or any core font.
func chooseFont(screenH int, setting string) fontSpec {
	if setting != "" {
		if size, err := strconv.Atoi(setting); err == nil {
			for _, f := range fixedFonts {
				if f.size == size {
					return f.fontSpec
				}
			}
		} else {
			return fontSpec{plain: setting, unicode: setting}
		}
	}
	spec := fixedFonts[0].fontSpec
	for _, f := range fixedFonts {
		if screenH >= f.minScreenH {
			spec = f.fontSpec
		}
	}
	return spec
}

type fontMetrics struct {
	fontSpec
	charW, ascent, descent int
}

// font is the font fion draws with, and its metrics: the smallest fixed
// until loadFont picks one for the screen.
var font = fontMetrics{fontSpec: fixedFonts[0].fontSpec, charW: 6, ascent: 11, descent: 2}

// the 12x24 font, for the bar above the largest fixed size
var sony24 = fontSpec{24, "-sony-fixed-medium-r-normal--24-170-100-100-c-120-iso8859-1", "", ""}

// barFontFor returns the fonts to try for the info bar, in order: one size
// up from the font, which reads better at the bottom of the screen.
func barFontFor(spec fontSpec) []fontSpec {
	for i, f := range fixedFonts {
		if f.fontSpec.plain != spec.plain {
			continue
		}
		if i+1 < len(fixedFonts) {
			return []fontSpec{fixedFonts[i+1].fontSpec, spec}
		}
		return []fontSpec{sony24, spec}
	}
	return []fontSpec{spec}
}

// barFont is the info bar's font, and its metrics.
var barFont = font

// loadFont picks the font for the screen and measures it, falling back to
// the smallest fixed font when it can't be opened, and the bar's.
func loadFont(conn *xgb.Conn, screenH int) {
	for _, spec := range []fontSpec{chooseFont(screenH, os.Getenv("FION_FONT")), fixedFonts[0].fontSpec} {
		if m, ok := measureFont(conn, spec); ok {
			font = m
			break
		}
	}
	barFont = font
	for _, spec := range barFontFor(font.fontSpec) {
		if m, ok := measureFont(conn, spec); ok {
			barFont = m
			return
		}
	}
}

// measureFont opens a font to read its metrics.
func measureFont(conn *xgb.Conn, spec fontSpec) (fontMetrics, bool) {
	fid, err := xproto.NewFontId(conn)
	if err != nil {
		return fontMetrics{}, false
	}
	if err := xproto.OpenFontChecked(conn, fid, uint16(len(spec.plain)), spec.plain).Check(); err != nil {
		log.Printf("font %q: %v", spec.plain, err)
		return fontMetrics{}, false
	}
	defer xproto.CloseFont(conn, fid)
	r, err := xproto.QueryFont(conn, xproto.Fontable(fid)).Reply()
	if err != nil {
		return fontMetrics{}, false
	}
	return fontMetrics{fontSpec: spec, charW: int(r.MaxBounds.CharacterWidth),
		ascent: int(r.FontAscent), descent: int(r.FontDescent)}, true
}

// openFont opens a font, reporting whether it could.
func openFont(conn *xgb.Conn, name string) (xproto.Font, bool) {
	if name == "" {
		return 0, false
	}
	fid, err := xproto.NewFontId(conn)
	if err != nil {
		return 0, false
	}
	return fid, xproto.OpenFontChecked(conn, fid, uint16(len(name)), name).Check() == nil
}

// sizes following the font
func charW() int { return font.charW }
func textH() int { return font.ascent + font.descent }

// titleH is the height of a title bar, and of the info bar, their border
// included; clients start below it.
func titleH() int { return textH() + 9 }

// baseline is where text sits in a box h pixels high.
func baseline(h int) int { return (h-textH())/2 + font.ascent }

// scaled scales a length drawn for the smallest font.
func scaled(v int) int { return v * textH() / 13 }
