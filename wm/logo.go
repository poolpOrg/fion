package wm

import (
	"bytes"
	"image"
	"image/jpeg"
	"log"
	"sync"

	"github.com/jezek/xgb/xproto"
	"github.com/poolpOrg/fion"
)

// Empty frames show the project's logo, centered: its luminance serves as
// a mask to draw it in colorLogo over colorEmpty.

// logoMask is how much of each pixel the logo covers, from 0 to 255.
type logoMask struct {
	w, h  int
	alpha []uint8
}

var (
	logoOnce sync.Once
	logo     *logoMask
)

// loadLogo decodes the embedded logo, once.
func loadLogo() *logoMask {
	logoOnce.Do(func() {
		img, err := jpeg.Decode(bytes.NewReader(fion.Logo))
		if err != nil {
			log.Printf("logo: %v", err)
			return
		}
		logo = maskOf(img)
	})
	return logo
}

// maskOf turns a light on dark image into a mask, cropped to the light.
func maskOf(img image.Image) *logoMask {
	b := img.Bounds()
	lum := func(x, y int) uint8 {
		r, g, bl, _ := img.At(x, y).RGBA()
		return uint8((299*r + 587*g + 114*bl) / 1000 >> 8)
	}

	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if lum(x, y) > 32 {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < minX {
		return &logoMask{}
	}

	m := &logoMask{w: maxX - minX + 1, h: maxY - minY + 1}
	m.alpha = make([]uint8, m.w*m.h)
	for y := range m.h {
		for x := range m.w {
			m.alpha[y*m.w+x] = lum(minX+x, minY+y)
		}
	}
	return m
}

// scaled returns the mask scaled to width w, each pixel the average of the
// ones it covers.
func (m *logoMask) scaled(w int) *logoMask {
	h := max(1, m.h*w/m.w)
	out := &logoMask{w: w, h: h, alpha: make([]uint8, w*h)}
	for ty := range h {
		sy0, sy1 := ty*m.h/h, max(ty*m.h/h+1, (ty+1)*m.h/h)
		for tx := range w {
			sx0, sx1 := tx*m.w/w, max(tx*m.w/w+1, (tx+1)*m.w/w)
			sum, n := 0, 0
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					sum += int(m.alpha[sy*m.w+sx])
					n++
				}
			}
			out.alpha[ty*w+tx] = uint8(sum / n)
		}
	}
	return out
}

// blend mixes the colors fg over bg, by a out of 255.
func blend(bg, fg uint32, a uint8) uint32 {
	var out uint32
	for shift := 0; shift <= 16; shift += 8 {
		b, f := (bg>>shift)&0xff, (fg>>shift)&0xff
		out |= ((b*(255-uint32(a)) + f*uint32(a)) / 255) << shift
	}
	return out
}

type logoImage struct {
	pixmap xproto.Pixmap
	w, h   int
}

// logoPixmap returns the logo rendered at width w, made once per width.
// It reports false when the screen isn't the 24-bit TrueColor fion draws
// for.
func (s *Screen) logoPixmap(w int) (logoImage, bool) {
	if img, ok := s.logos[w]; ok {
		return img, true
	}
	m := loadLogo()
	scr := s.Info()
	if m == nil || m.w == 0 || scr.RootDepth != 24 {
		return logoImage{}, false
	}
	setup := s.wm.Setup
	bpp := 0
	for _, f := range setup.PixmapFormats {
		if f.Depth == 24 {
			bpp = int(f.BitsPerPixel)
		}
	}
	if bpp != 32 {
		return logoImage{}, false
	}

	sm := m.scaled(w)
	conn := s.Conn()
	pm, err := xproto.NewPixmapId(conn)
	if err != nil {
		return logoImage{}, false
	}
	xproto.CreatePixmap(conn, 24, pm, xproto.Drawable(scr.Root), uint16(sm.w), uint16(sm.h))
	if s.logoGC == 0 {
		gc, err := xproto.NewGcontextId(conn)
		if err != nil {
			return logoImage{}, false
		}
		xproto.CreateGC(conn, gc, xproto.Drawable(scr.Root), xproto.GcGraphicsExposures, []uint32{0})
		s.logoGC = gc
	}

	// in bands of rows that fit in a request
	rowBytes := sm.w * 4
	rows := max(1, (int(setup.MaximumRequestLength)*4-32)/rowBytes)
	for y0 := 0; y0 < sm.h; y0 += rows {
		n := min(rows, sm.h-y0)
		data := make([]byte, 0, n*rowBytes)
		for _, a := range sm.alpha[y0*sm.w : (y0+n)*sm.w] {
			c := blend(colorEmpty, colorLogo, a)
			r, g, b := byte(c>>16), byte(c>>8), byte(c)
			if setup.ImageByteOrder == xproto.ImageOrderLSBFirst {
				data = append(data, b, g, r, 0)
			} else {
				data = append(data, 0, r, g, b)
			}
		}
		xproto.PutImage(conn, xproto.ImageFormatZPixmap, xproto.Drawable(pm), s.logoGC,
			uint16(sm.w), uint16(n), 0, int16(y0), 0, 24, data)
	}

	img := logoImage{pixmap: pm, w: sm.w, h: sm.h}
	if s.logos == nil {
		s.logos = map[int]logoImage{}
	}
	s.logos[w] = img
	return img, true
}

// drawLogo draws the logo centered in f when it holds no client.
func (f *Frame) drawLogo() {
	if !f.leaf || len(f.clients) != 0 {
		return
	}
	m := loadLogo()
	if m == nil || m.w == 0 {
		return
	}
	areaW, areaH := int(f.g.W), int(f.g.H)-22
	w := min(areaW*45/100, 720)
	if h := m.h * w / m.w; h > areaH*6/10 {
		w = w * areaH * 6 / 10 / h
	}
	if w < 32 {
		return
	}
	img, ok := f.screen.logoPixmap(w)
	if !ok {
		return
	}
	xproto.CopyArea(f.Conn(), xproto.Drawable(img.pixmap), xproto.Drawable(f.window), f.screen.logoGC,
		0, 0, int16((areaW-img.w)/2), int16(22+(areaH-img.h)/2), uint16(img.w), uint16(img.h))
}

// frameByWindow finds a frame of the active screen by its window.
func (wm *Manager) frameByWindow(win xproto.Window) *Frame {
	s := wm.GetActiveScreen()
	if s == nil {
		return nil
	}
	if s.scratchpad != nil && s.scratchpad.window == win {
		return s.scratchpad
	}
	var found *Frame
	for _, ws := range s.Workspaces {
		walk(ws.Root, func(f *Frame) {
			if f.window == win {
				found = f
			}
		})
	}
	return found
}
