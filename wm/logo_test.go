package wm

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestLogoMask(t *testing.T) {
	m := loadLogo()
	if m == nil || m.w == 0 {
		t.Fatalf("logo didn't decode")
	}
	// cropped to the drawing, which is wider than tall
	if m.w >= 2000 || m.h >= 2000 || m.w <= m.h {
		t.Fatalf("logo mask %dx%d, want cropped to the wide drawing", m.w, m.h)
	}
	covered := 0
	for _, a := range m.alpha {
		if a > 128 {
			covered++
		}
	}
	if covered == 0 {
		t.Fatalf("logo mask is empty")
	}

	s := m.scaled(400)
	if s.w != 400 || s.h != m.h*400/m.w || len(s.alpha) != s.w*s.h {
		t.Fatalf("scaled mask %dx%d (%d values)", s.w, s.h, len(s.alpha))
	}

	if blend(0x000000, 0xffffff, 0) != 0x000000 || blend(0x000000, 0xffffff, 255) != 0xffffff {
		t.Fatalf("blend at the ends")
	}
	if c := blend(colorEmpty, colorLogo, 255); c != colorLogo {
		t.Fatalf("blend fully covered = %06x, want %06x", c, colorLogo)
	}
}

// logoPixels counts the pixels of the logo's color in the middle band of
// f, below its title bar.
func logoPixels(t *testing.T, f *Frame) int {
	t.Helper()
	y := int16(titleH() + (int(f.g.H)-titleH())/2 - 50)
	img, err := xproto.GetImage(f.Conn(), xproto.ImageFormatZPixmap, xproto.Drawable(f.window),
		0, y, f.g.W, 100, ^uint32(0)).Reply()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for i := 0; i+3 < len(img.Data); i += 4 {
		c := uint32(img.Data[i+2])<<16 | uint32(img.Data[i+1])<<8 | uint32(img.Data[i])
		if c == colorLogo {
			n++
		}
	}
	return n
}

func TestLogoOnlyInSingleEmptyFrame(t *testing.T) {
	wm := newTestManager(t)
	drainEvents(t, wm)
	ws := wm.GetActiveWorkspace()
	if n := logoPixels(t, ws.Root); n < 100 {
		t.Fatalf("%d pixels of the logo in a workspace that is a single empty frame", n)
	}

	// split: no logo in either half
	if err := ws.splitV(); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	for _, f := range ws.Root.children {
		if n := logoPixels(t, f); n != 0 {
			t.Fatalf("%d pixels of the logo in a half of a split workspace", n)
		}
	}

	// back to a single frame: the logo again
	removeActive(t, ws)
	drainEvents(t, wm)
	if n := logoPixels(t, ws.Root); n < 100 {
		t.Fatalf("%d pixels of the logo once back to a single frame", n)
	}

	// none in the empty scratchpad either
	s := wm.GetActiveScreen()
	if err := s.toggleScratchpad(); err != nil {
		t.Fatal(err)
	}
	drainEvents(t, wm)
	if n := logoPixels(t, s.scratchpad); n != 0 {
		t.Fatalf("%d pixels of the logo in the empty scratchpad", n)
	}
}
