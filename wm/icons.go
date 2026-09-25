package wm

import "math"

// The info bar's small icons for the operating systems, drawn from shapes
// rather than taken from the systems' logos. Colors are Dracula's.

const (
	draculaRed    = 0xff5555
	draculaOrange = 0xffb86c
	draculaYellow = 0xf1fa8c
	iconInk       = 0x000000 // eyes
	transparent   = 1 << 24  // a pixel left to the background
)

// icon is a small image, its pixels 0xRRGGBB colors or transparent.
type icon struct {
	w, h int
	px   []uint32
}

func newIcon(w, h int) *icon {
	ic := &icon{w: w, h: h, px: make([]uint32, w*h)}
	for i := range ic.px {
		ic.px[i] = transparent
	}
	return ic
}

func (ic *icon) set(x, y int, c uint32) {
	if x >= 0 && y >= 0 && x < ic.w && y < ic.h {
		ic.px[y*ic.w+x] = c
	}
}

// ellipse fills the ellipse centered on cx, cy, sampled at pixel centers.
func (ic *icon) ellipse(cx, cy, rx, ry float64, c uint32) {
	for y := range ic.h {
		for x := range ic.w {
			dx, dy := (float64(x)+0.5-cx)/rx, (float64(y)+0.5-cy)/ry
			if dx*dx+dy*dy <= 1 {
				ic.set(x, y, c)
			}
		}
	}
}

func (ic *icon) rect(x0, y0, x1, y1 int, c uint32) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			ic.set(x, y, c)
		}
	}
}

// pixel returns the color of a pixel, the background where transparent.
func (ic *icon) pixel(x, y int, background uint32) uint32 {
	if c := ic.px[y*ic.w+x]; c != transparent {
		return c
	}
	return background
}

// osIcon draws the icon for an operating system, 14 pixels high.
func osIcon(kind string) *icon {
	switch kind {
	case "openbsd":
		return blowfishIcon()
	case "linux":
		return penguinIcon()
	case "freebsd":
		return hornedIcon()
	case "netbsd":
		return flagIcon()
	}
	return monitorIcon()
}

// blowfishIcon is a puffed up blowfish, spiky, facing right.
func blowfishIcon() *icon {
	ic := newIcon(18, 14)
	// the tail, widening away from the body
	for x, half := range []int{3, 2, 2, 1, 1} {
		ic.rect(x, 7-half, x, 6+half, draculaYellow)
	}
	// spikes around the body, but on the tail's side
	for a := 0.0; a < 360; a += 30 {
		if a > 120 && a < 240 {
			continue
		}
		r := a * math.Pi / 180
		ic.set(int(10.5+6.6*math.Cos(r)), int(7+6.2*math.Sin(r)), draculaOrange)
	}
	ic.ellipse(10.5, 7, 5.6, 5.3, draculaYellow)
	// the eye, and the mouth
	ic.rect(12, 4, 13, 5, draculaForeground)
	ic.set(13, 4, iconInk)
	ic.set(15, 8, draculaOrange)
	return ic
}

// penguinIcon is a penguin standing, its belly white.
func penguinIcon() *icon {
	ic := newIcon(12, 14)
	const body = 0x6272a4
	ic.ellipse(6, 3.5, 3.2, 3.2, body) // head
	ic.ellipse(6, 8.5, 4.5, 4.8, body) // body
	ic.ellipse(6, 9, 3, 3.8, draculaForeground)
	ic.set(5, 3, draculaForeground)
	ic.set(7, 3, draculaForeground)
	ic.rect(5, 5, 7, 5, draculaOrange) // the beak
	ic.rect(2, 13, 4, 13, draculaOrange)
	ic.rect(7, 13, 9, 13, draculaOrange)
	return ic
}

// hornedIcon is a red ball with two horns.
func hornedIcon() *icon {
	ic := newIcon(14, 14)
	ic.ellipse(7, 8, 5.5, 5.5, draculaRed)
	for i := range 3 {
		ic.set(1+i, 1+i, draculaRed)
		ic.set(2+i, 1+i, draculaRed)
		ic.set(12-i, 1+i, draculaRed)
		ic.set(11-i, 1+i, draculaRed)
	}
	ic.rect(4, 5, 5, 6, draculaForeground) // a shine
	return ic
}

// flagIcon is a flag on its pole.
func flagIcon() *icon {
	ic := newIcon(14, 14)
	ic.rect(1, 0, 1, 13, draculaForeground)
	for x := 2; x < 13; x++ {
		// waving
		dy := int(math.Round(math.Sin(float64(x) / 2)))
		ic.rect(x, 1+dy, x, 7+dy, draculaOrange)
	}
	return ic
}

// monitorIcon is a screen on its stand, for the other systems.
func monitorIcon() *icon {
	ic := newIcon(14, 14)
	ic.rect(0, 1, 13, 10, draculaForeground)
	ic.rect(1, 2, 12, 9, draculaComment)
	ic.rect(6, 11, 7, 11, draculaForeground)
	ic.rect(3, 12, 10, 12, draculaForeground)
	return ic
}
