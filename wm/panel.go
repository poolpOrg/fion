package wm

import (
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/jezek/xgb/xproto"
)

// The expanded info bar: a panel above the bar, shown and hidden by
// clicking the bar or with Mod+s, with the machine's hardware, CPUs and
// temperatures, GPUs, memory, and disk and network throughput, refreshed
// every second.

const (
	draculaGreen = 0x50fa7b
	draculaCyan  = 0x8be9fd

	panelPad = 12
)

// the panel's sizes, following the font
func panelLineH() int  { return textH() + 2 }
func panelMeterW() int { return scaled(110) }
func panelMeterH() int { return max(4, textH()*8/13) }

type sysPanel struct {
	screen *Screen
	window xproto.Window
	gc     xproto.Gcontext
	boldGC xproto.Gcontext // headers, the plain font when bold is missing
	shown  bool
	h      int

	sampler sampler
	snap    sysSnapshot
}

// panelHeight is the panel's height on a screen of height screenH, for a
// machine with cores CPUs: all of them in a column, but at most 60% of
// the screen.
func panelHeight(screenH, cores int) int {
	rows := max(cores+6, 22)
	return min(screenH*6/10, rows*panelLineH()+2*panelPad)
}

func newSysPanel(s *Screen) (*sysPanel, error) {
	conn := s.Conn()
	g := s.Geometry()
	p := &sysPanel{screen: s}
	p.snap = p.sampler.sample()
	p.h = panelHeight(int(g.H), len(p.snap.cpus))

	w, err := xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}
	xproto.CreateWindow(conn, s.Info().RootDepth, w, s.Info().Root,
		0, int16(int(g.H)-infoBarOuterH()-p.h), g.W, uint16(p.h), 0,
		xproto.WindowClassInputOutput, s.Info().RootVisual,
		xproto.CwBackPixel|xproto.CwEventMask,
		[]uint32{colorBar, xproto.EventMaskExposure | xproto.EventMaskButtonPress})
	p.window = w

	newGC := func(fid xproto.Font) xproto.Gcontext {
		gc, _ := xproto.NewGcontextId(conn)
		xproto.CreateGC(conn, gc, xproto.Drawable(w), xproto.GcForeground|xproto.GcBackground|xproto.GcFont,
			[]uint32{colorText, colorBar, uint32(fid)})
		xproto.CloseFont(conn, fid)
		return gc
	}
	plain, _ := openFont(conn, font.plain)
	p.gc = newGC(plain)
	// the plain font is closed once in its GC: share that GC when the bold
	// font is missing
	p.boldGC = p.gc
	if bold, ok := openFont(conn, font.bold); ok {
		p.boldGC = newGC(bold)
	}
	return p, nil
}

// togglePanel shows the panel, creating it the first time, or hides it.
func (s *Screen) togglePanel() error {
	if s.panel == nil {
		p, err := newSysPanel(s)
		if err != nil {
			return err
		}
		s.panel = p
	}
	p := s.panel
	p.shown = !p.shown
	if !p.shown {
		xproto.UnmapWindow(s.Conn(), p.window)
		return nil
	}
	p.snap = p.sampler.sample()
	xproto.MapWindow(s.Conn(), p.window)
	xproto.ConfigureWindow(s.Conn(), p.window, xproto.ConfigWindowStackMode, []uint32{xproto.StackModeAbove})
	p.draw()
	return nil
}

// refresh samples the machine again and redraws, while shown.
func (p *sysPanel) refresh() {
	if !p.shown {
		return
	}
	p.snap = p.sampler.sample()
	p.draw()
}

// meterColor is the color of a meter at pct percent.
func meterColor(pct float64) uint32 {
	switch {
	case pct >= 80:
		return draculaRed
	case pct >= 50:
		return draculaYellow
	}
	return draculaGreen
}

// formatUptime shows a duration in days, hours and minutes.
func formatUptime(d time.Duration) string {
	d = d.Round(time.Minute)
	days, hours, minutes := int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func formatRate(bps float64) string {
	return humanize.IBytes(uint64(bps)) + "/s"
}

// activeRates returns the rates that moved bytes, and how many didn't.
func activeRates(rs []ioRate) (busy []ioRate, idle int) {
	for _, r := range rs {
		if r.in+r.out > 0 {
			busy = append(busy, r)
		} else {
			idle++
		}
	}
	return busy, idle
}

func percent(part, whole uint64) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) * 100 / float64(whole)
}

// column draws lines of text and meters top to bottom in a column.
type column struct {
	p      *sysPanel
	x, y   int
	w      int
	bottom int
}

func (c *column) room() bool {
	return c.y+panelLineH() <= c.bottom
}

func (c *column) text(x int, s string, fg uint32, bold bool) {
	if len(s) > 255 {
		s = s[:255]
	}
	gc := c.p.gc
	if bold {
		gc = c.p.boldGC
	}
	conn := c.p.screen.Conn()
	xproto.ChangeGC(conn, gc, xproto.GcForeground, []uint32{fg})
	xproto.ImageText8(conn, byte(len(s)), xproto.Drawable(c.p.window), gc, int16(x), int16(c.y+font.ascent), s)
}

func (c *column) header(s string) {
	if !c.room() {
		return
	}
	c.text(c.x, s, colorAccent, true)
	c.y += panelLineH()
}

func (c *column) line(s string, fg uint32) {
	if !c.room() {
		return
	}
	if n := c.w / charW(); len(s) > n {
		s = s[:max(n, 0)]
	}
	c.text(c.x, s, fg, false)
	c.y += panelLineH()
}

// meter draws a labelled meter, colored by how high it is, then text
// after it.
func (c *column) meter(label string, pct float64, after string) {
	c.coloredMeter(label, pct, after, meterColor(pct))
}

// coloredMeter draws a labelled meter in color, then text after it.
func (c *column) coloredMeter(label string, pct float64, after string, color uint32) {
	if !c.room() {
		return
	}
	c.text(c.x, label, colorText, false)
	mx := c.x + (len(label)+1)*charW()
	conn := c.p.screen.Conn()
	fill := func(x, w int, color uint32) {
		xproto.ChangeGC(conn, c.p.gc, xproto.GcForeground, []uint32{color})
		xproto.PolyFillRectangle(conn, xproto.Drawable(c.p.window), c.p.gc,
			[]xproto.Rectangle{{X: int16(x), Y: int16(c.y + (panelLineH()-panelMeterH())/2), Width: uint16(w), Height: uint16(panelMeterH())}})
	}
	fill(mx, panelMeterW(), colorTab)
	if w := int(min(100, max(0, pct)) * float64(panelMeterW()) / 100); w > 0 {
		fill(mx, w, color)
	}
	c.text(mx+panelMeterW()+charW(), after, colorText, false)
	c.y += panelLineH()
}

func (c *column) gap() {
	c.y += panelLineH() / 2
}

func (p *sysPanel) draw() {
	conn := p.screen.Conn()
	width := int(p.screen.Geometry().W)
	xproto.ClearArea(conn, false, p.window, 0, 0, 0, 0)
	// a line on top, to part the panel from the frames
	xproto.ChangeGC(conn, p.gc, xproto.GcForeground, []uint32{colorAccent})
	xproto.PolyFillRectangle(conn, xproto.Drawable(p.window), p.gc,
		[]xproto.Rectangle{{X: 0, Y: 0, Width: uint16(width), Height: 1}})

	colW := (width - 4*panelPad) / 3
	col := func(i int) *column {
		return &column{p: p, x: panelPad + i*(colW+panelPad), y: panelPad, w: colW, bottom: p.h - panelPad/2}
	}
	s := p.snap

	// hardware, then memory
	c := col(0)
	h := s.host
	c.header("HARDWARE")
	for _, kv := range [][2]string{
		{"host", h.hostname},
		{"machine", h.model},
		{"cpu", fmt.Sprintf("%s, %d threads", h.cpuModel, h.cores)},
		{"memory", humanize.IBytes(h.memTotal)},
		{"system", thisOS().String()},
		{"kernel", h.kernel},
		{"uptime", formatUptime(time.Since(h.boot))},
	} {
		if kv[1] != "" && !strings.HasPrefix(kv[1], ", ") {
			c.line(fmt.Sprintf("%-8s %s", kv[0], kv[1]), colorText)
		}
	}
	c.gap()
	c.header("MEMORY")
	m := s.mem
	for _, row := range []struct {
		label string
		value uint64
	}{
		{"used     ", m.used}, {"available", m.available}, {"cached   ", m.cached}, {"free     ", m.free},
	} {
		if row.value > 0 || row.label == "used     " {
			pct := percent(row.value, m.total)
			after := fmt.Sprintf("%9s %3.0f%%", humanize.IBytes(row.value), pct)
			// only the memory used tells of pressure
			if row.label == "used     " {
				c.meter(row.label, pct, after)
			} else {
				c.coloredMeter(row.label, pct, after, draculaCyan)
			}
		}
	}
	if m.swapTotal > 0 {
		pct := percent(m.swapUsed, m.swapTotal)
		c.meter("swap     ", pct, fmt.Sprintf("%9s of %s", humanize.IBytes(m.swapUsed), humanize.IBytes(m.swapTotal)))
	}

	// CPUs, then GPUs
	c = col(1)
	c.header("CPU")
	c.meter("all ", s.cpuTotal, fmt.Sprintf("%3.0f%%", s.cpuTotal))
	// the cores, in two columns when they don't fit in one
	rows := (c.bottom - c.y) / panelLineH()
	reserve := 5 // for the GPU below
	split := len(s.cpus) > rows-reserve
	half := (len(s.cpus) + 1) / 2
	top := c.y
	for i, pct := range s.cpus {
		label := fmt.Sprintf("C%-3d", i)
		after := fmt.Sprintf("%3.0f%%", pct)
		if t, ok := s.coreTemps[i]; ok {
			after += fmt.Sprintf(" %3.0f\xb0C", t)
		}
		if split && i == half {
			c.y = top
			c.x += colW / 2
		}
		c.meter(label, pct, after)
	}
	if split {
		c.x -= colW / 2
		c.y = top + half*panelLineH()
	}
	if n := len(s.cpuTemps); n > 0 && len(s.coreTemps) == 0 {
		c.line(fmt.Sprintf("temp %.0f-%.0f\xb0C over %d sensors", s.cpuTemps[0], s.cpuTemps[n-1], n), colorText)
	}
	c.gap()
	c.header("GPU")
	if len(s.gpus) == 0 {
		c.line("no information on this system", colorDim)
	}
	for _, g := range s.gpus {
		c.line(g.name, colorText)
		after := ""
		if g.util >= 0 {
			after = fmt.Sprintf("%3.0f%%", g.util)
		}
		if g.memUsed > 0 {
			after += "  mem " + humanize.IBytes(g.memUsed)
			if g.memTotal > 0 {
				after += " of " + humanize.IBytes(g.memTotal)
			}
		}
		if g.temp >= 0 {
			after += fmt.Sprintf("  %.0f\xb0C", g.temp)
		}
		c.meter("use ", max(g.util, 0), after)
	}

	// disks, network, other sensors
	c = col(2)
	c.header("DISK I/O")
	ioLines := func(rs []ioRate, in, out string) {
		if rs == nil {
			c.line("measuring...", colorDim)
			return
		}
		busy, idle := activeRates(rs)
		for _, r := range busy {
			c.line(fmt.Sprintf("%-8s %s %-12s %s %s", r.name, in, formatRate(r.in), out, formatRate(r.out)), colorText)
		}
		if idle > 0 {
			c.line(fmt.Sprintf("%d idle", idle), colorDim)
		}
	}
	ioLines(s.disks, "read", "write")
	c.gap()
	c.header("NETWORK")
	ioLines(s.nets, "in", "out")
	if len(s.sensors) > 0 {
		c.gap()
		c.header("SENSORS")
		for _, t := range s.sensors {
			c.line(fmt.Sprintf("%-20s %5.1f\xb0C", t.name, t.temp), colorText)
		}
	}
}
