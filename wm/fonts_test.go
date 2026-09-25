package wm

import (
	"strings"
	"testing"
)

func TestChooseFont(t *testing.T) {
	for _, tc := range []struct {
		screenH int
		setting string
		size    int
	}{
		{800, "", 13}, {1080, "", 15}, {1600, "", 18}, {2160, "", 20},
		{800, "18", 18}, {2160, "13", 13},
		{800, "99", 13}, // no such size: the screen's
	} {
		if got := chooseFont(tc.screenH, tc.setting); got.size != tc.size {
			t.Errorf("chooseFont(%d, %q) is size %d, want %d", tc.screenH, tc.setting, got.size, tc.size)
		}
	}
	custom := chooseFont(800, "-misc-fixed-medium-r-normal--14-130-75-75-c-70-iso8859-1")
	if custom.plain != "-misc-fixed-medium-r-normal--14-130-75-75-c-70-iso8859-1" || custom.bold != "" {
		t.Fatalf("custom font %+v", custom)
	}
	for _, f := range fixedFonts {
		if !strings.HasSuffix(f.unicode, "iso10646-1") {
			t.Errorf("size %d has no Unicode font for xterm", f.size)
		}
	}
}

func TestSizesFollowFont(t *testing.T) {
	saved := font
	t.Cleanup(func() { font = saved })

	// the smallest font keeps the sizes fion always had
	font = fontMetrics{fontSpec: fixedFonts[0].fontSpec, charW: 6, ascent: 11, descent: 2}
	if titleH() != 22 || infoBarOuterH() != 20 || baseline(titleH()-2) != 14 || scaled(640) != 640 {
		t.Fatalf("sizes for 6x13: title %d bar %d baseline %d", titleH(), infoBarOuterH(), baseline(titleH()-2))
	}
	// 10x20 grows them
	font = fontMetrics{fontSpec: fixedFonts[3].fontSpec, charW: 10, ascent: 16, descent: 4}
	if titleH() <= 22 || infoBarOuterH() <= 20 || scaled(640) <= 640 || !strings.Contains(xtermTheme()[0], "--20-") {
		t.Fatalf("sizes for 10x20: title %d bar %d, xterm %q", titleH(), infoBarOuterH(), xtermTheme()[0])
	}
}
