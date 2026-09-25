package wm

import (
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/jezek/xgb/xproto"
)

// The expanded info bar: a panel above the bar, shown and hidden by
// clicking the bar or with Mod+s, refreshed every second. Its summary shows
// the machine's hardware, CPUs and temperatures, GPUs, memory, filesystems,
// and disk and network throughput; its other views, cycled with Tab and the
// arrows, detail each with graphs of the last minutes.

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
	hist    histories

	view int      // into panelViews
	tabs [][2]int // the view names' spans, for clicks

	// the processes using the most CPU and the ports listened to,
	// gathered away from the event loop while their view is shown
	procs     []procUsage
	ports     []listenPort
	portsRead bool
	procS     procSampler
	gathering bool
	gathered  time.Time
}

var panelViews = []string{"Summary", "CPU", "Memory", "Disk", "Network", "Sensors", "Ports", "Messages"}

// the panel's height in lines, at least what the graphs need, at most
// half the screen
const panelMinLines = 12

func panelMaxLines(screenH int) int {
	return max(panelMinLines, (screenH/2-panelChromeH())/panelLineH())
}

// panelChromeH is the panel's height but for its lines: the views' names
// and the space around them.
func panelChromeH() int { return panelPad/2 + panelLineH() + panelPad/2 + panelPad/2 }

func newSysPanel(s *Screen) (*sysPanel, error) {
	conn := s.Conn()
	g := s.Geometry()
	p := &sysPanel{screen: s, hist: histories{}}
	p.snap = p.sampler.sample()
	p.h = p.fitHeight()

	w, err := xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}
	xproto.CreateWindow(conn, s.Info().RootDepth, w, s.Info().Root,
		g.X, g.Y+int16(int(g.H)-infoBarOuterH()-p.h), g.W, uint16(p.h), 0,
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
	if p.shown {
		p.shown = false
		xproto.UnmapWindow(s.Conn(), p.window)
		s.layoutWorkspaces()
		s.wm.drawNotifications()
		s.wm.KeyboardManager.UngrabKeyboard()
		return nil
	}
	// its keys, while shown
	if err := s.wm.KeyboardManager.GrabKeyboard(s.Info().Root); err != nil {
		return err
	}
	p.shown = true
	defer s.wm.drawNotifications()
	p.snap = p.sampler.sample()
	p.hist.record(p.snap)
	// as high as the summary needs, the frames above it
	p.h = p.fitHeight()
	g := s.Geometry()
	xproto.ConfigureWindow(s.Conn(), p.window,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
		[]uint32{uint32(g.X), uint32(int(g.Y) + int(g.H) - infoBarOuterH() - p.h), uint32(g.W), uint32(p.h)})
	s.layoutWorkspaces()
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
	p.hist.record(p.snap)
	p.gather()
	p.draw()
}

// gather samples the processes, or the ports, for the view shown, every
// two seconds, away from the event loop.
func (p *sysPanel) gather() {
	view := panelViews[p.view]
	if (view != "CPU" && view != "Ports") || p.gathering || time.Since(p.gathered) < 2*time.Second {
		return
	}
	p.gathering = true
	wm := p.screen.wm
	work := func() func() {
		var procs []procUsage
		var ports []listenPort
		if view == "CPU" {
			procs = p.procS.top(16)
		} else {
			ports = listeningPorts()
		}
		return func() {
			if view == "CPU" {
				p.procs = procs
			} else {
				p.ports, p.portsRead = ports, true
			}
			p.gathering, p.gathered = false, time.Now()
			if p.shown {
				p.draw()
			}
		}
	}
	if wm.later == nil {
		// no event loop, as in tests
		work()()
		return
	}
	go func() { wm.later <- work() }()
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
	// without a background, over highlights and graphs; the panel is
	// cleared before it is drawn
	for len(s) > 0 {
		n := min(len(s), 254)
		xproto.PolyText8(conn, xproto.Drawable(c.p.window), gc, int16(x), int16(c.y+font.ascent),
			append([]byte{byte(n), 0}, s[:n]...))
		x += n * charW()
		s = s[n:]
	}
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
	p.fill(0, 0, width, 1, colorAccent)

	// the views' names, the one shown highlighted
	p.tabs = p.tabs[:0]
	c := &column{p: p, x: panelPad, y: panelPad / 2, w: width, bottom: p.h}
	x := panelPad
	for i, name := range panelViews {
		label := fmt.Sprintf(" %d %s ", i+1, name)
		w := len(label) * charW()
		if i == p.view {
			p.fill(x, c.y, w, panelLineH(), colorAccent)
			c.text(x, label, colorAccentText, true)
		} else {
			c.text(x, label, colorDim, false)
		}
		p.tabs = append(p.tabs, [2]int{x, x + w})
		x += w + charW()
	}
	c.text(x+2*charW(), fmt.Sprintf("Tab, arrows or 1-%d to switch, Escape closes", len(panelViews)), colorDim, false)

	top := panelPad/2 + panelLineH() + panelPad/2
	area := rect{panelPad, top, width - 2*panelPad, p.h - top - panelPad/2}
	switch panelViews[p.view] {
	case "Summary":
		p.drawSummary(area)
	case "CPU":
		p.drawCPU(area)
	case "Memory":
		p.drawMemory(area)
	case "Disk":
		p.drawDisk(area)
	case "Network":
		p.drawNetwork(area)
	case "Sensors":
		p.drawSensors(area)
	case "Ports":
		p.drawPorts(area)
	case "Messages":
		p.drawMessages(area)
	}
}

func (p *sysPanel) fill(x, y, w, h int, color uint32) {
	if w <= 0 || h <= 0 {
		return
	}
	conn := p.screen.Conn()
	xproto.ChangeGC(conn, p.gc, xproto.GcForeground, []uint32{color})
	xproto.PolyFillRectangle(conn, xproto.Drawable(p.window), p.gc,
		[]xproto.Rectangle{{X: int16(x), Y: int16(y), Width: uint16(w), Height: uint16(h)}})
}

// graph draws the history key in r, as columns two pixels wide, against
// top, or its peak when top is 0, colored by color, and returns the top of
// its scale.
func (p *sysPanel) graph(r rect, key string, top float64, color func(v, top float64) uint32) float64 {
	p.fill(r.x, r.y, r.w, r.h, colorTab)
	const step = 2
	vals := p.hist[key].last(r.w / step)
	cols, top := graphColumns(vals, r.w, r.h, step, top)
	byColor := map[uint32][]xproto.Rectangle{}
	x0 := r.x + r.w - len(cols)*step
	for i, h := range cols {
		if h == 0 {
			continue
		}
		c := color(vals[i], top)
		byColor[c] = append(byColor[c], xproto.Rectangle{
			X: int16(x0 + i*step), Y: int16(r.y + r.h - h), Width: step, Height: uint16(h)})
	}
	conn := p.screen.Conn()
	for c, rects := range byColor {
		xproto.ChangeGC(conn, p.gc, xproto.GcForeground, []uint32{c})
		xproto.PolyFillRectangle(conn, xproto.Drawable(p.window), p.gc, rects)
	}
	return top
}

// graph colors: by level for percentages and temperatures, flat for rates
func byLevel(v, _ float64) uint32 { return meterColor(v) }
func byTemp(v, _ float64) uint32 {
	switch {
	case v >= 80:
		return draculaRed
	case v >= 60:
		return draculaYellow
	}
	return draculaGreen
}
func flat(c uint32) func(float64, float64) uint32 { return func(float64, float64) uint32 { return c } }

const draculaPink = 0xff79c6

// label draws text at x, y in r's corner, over a graph.
func (p *sysPanel) label(x, y int, s string, fg uint32) {
	c := &column{p: p, x: x, y: y, w: 1 << 20, bottom: 1 << 20}
	c.text(x, s, fg, false)
}

// ioLine shows a disk's or an interface's throughput, idle ones dimmed.
func ioLine(c rowWriter, name string, rs []ioRate, in, out string) {
	r, ok := rateOf(rs, name)
	// as wide whatever it shows, the summary laid out for the widest
	busy := func(a, b string) string { return fmt.Sprintf("%-8s %s %-12s %s %-12s", name, in, a, out, b) }
	n := len(busy("", ""))
	switch {
	case !ok:
		c.line(fmt.Sprintf("%-*s", n, fmt.Sprintf("%-8s measuring...", name)), colorDim)
	case r.in+r.out == 0:
		c.line(fmt.Sprintf("%-*s", n, fmt.Sprintf("%-8s idle", name)), colorDim)
	default:
		c.line(busy(formatRate(r.in), formatRate(r.out)), colorText)
	}
}

// A summary row: a line of text or a meter, a section's header, or the
// space between sections.
type summaryRow struct {
	kind int
	w    int // in pixels
	draw func(c *column)
}

const (
	rowLine = iota
	rowHeader
	rowGap
)

// rowCollector gathers the summary's rows, the column's methods measuring
// them instead of drawing.
type rowCollector struct{ rows []summaryRow }

func (r *rowCollector) header(s string) {
	if len(r.rows) > 0 {
		r.rows = append(r.rows, summaryRow{kind: rowGap})
	}
	r.rows = append(r.rows, summaryRow{kind: rowHeader, w: len(s) * charW(), draw: func(c *column) { c.header(s) }})
}

func (r *rowCollector) line(s string, fg uint32) {
	r.rows = append(r.rows, summaryRow{w: len(s) * charW(), draw: func(c *column) { c.line(s, fg) }})
}

func meterRowW(label, after string) int {
	return (len(label)+2+len(after))*charW() + panelMeterW()
}

func (r *rowCollector) meter(label string, pct float64, after string) {
	r.rows = append(r.rows, summaryRow{w: meterRowW(label, after), draw: func(c *column) { c.meter(label, pct, after) }})
}

func (r *rowCollector) coloredMeter(label string, pct float64, after string, color uint32) {
	r.rows = append(r.rows, summaryRow{w: meterRowW(label, after),
		draw: func(c *column) { c.coloredMeter(label, pct, after, color) }})
}

// summaryRows are the summary's sections, row by row.
func (p *sysPanel) summaryRows() []summaryRow {
	r := &rowCollector{}
	s := p.snap
	h := s.host
	r.header("HARDWARE")
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
			r.line(fmt.Sprintf("%-8s %s", kv[0], kv[1]), colorText)
		}
	}
	r.header("MEMORY")
	p.memoryMeters(r)
	r.header("FILESYSTEMS")
	p.filesystemMeters(r, 9)

	r.header("CPU")
	r.meter("all ", s.cpuTotal, fmt.Sprintf("%3.0f%%  load %.2f %.2f %.2f", s.cpuTotal, s.load[0], s.load[1], s.load[2]))
	for i, pct := range s.cpus {
		after := fmt.Sprintf("%3.0f%%", pct)
		if t, ok := s.coreTemps[i]; ok {
			after += fmt.Sprintf(" %3.0f\xb0C", t)
		}
		r.meter(fmt.Sprintf("C%-3d", i), pct, after)
	}
	if n := len(s.cpuTemps); n > 0 && len(s.coreTemps) == 0 {
		r.line(fmt.Sprintf("temp %.0f-%.0f\xb0C over %d sensors", s.cpuTemps[0], s.cpuTemps[n-1], n), colorText)
	}
	r.header("GPU")
	p.gpuLines(r)

	// all of them, idle ones marked
	r.header("DISK I/O")
	for _, name := range ioNames(s.diskTotals) {
		ioLine(r, name, s.disks, "read", "write")
	}
	r.header("NETWORK")
	for _, name := range ioNames(s.netTotals) {
		ioLine(r, name, s.nets, "in", "out")
	}
	if len(s.sensors) > 0 {
		r.header("SENSORS")
		for _, t := range s.sensors {
			r.line(fmt.Sprintf("%-20s %5.1f\xb0C", t.name, t.temp), colorText)
		}
	}
	return r.rows
}

// a column of the summary: where it starts, its rows
type summaryColumn struct {
	x    int
	rows []summaryRow
}

// flowSummary flows rows into columns lines high, each as wide as its
// widest row, and returns them and the width they take.
func flowSummary(rows []summaryRow, lines int) ([]summaryColumn, int) {
	lh := panelLineH()
	space := 4 * charW()
	capacity := lines * lh
	var cols []summaryColumn
	x, y, colW := 0, capacity, 0 // as if a column were full, to start one
	for i, r := range rows {
		rh := lh
		if r.kind == rowGap {
			rh = lh / 2
		}
		need := rh
		if r.kind == rowHeader {
			// the whole section when it fits in a column, its first row
			// with it otherwise
			need = sectionH(rows[i:])
			if need > capacity {
				need = min(2, len(rows)-i) * lh
			}
		}
		if y+need > capacity {
			if len(cols) > 0 {
				x += colW + space
			}
			cols = append(cols, summaryColumn{x: x})
			y, colW = 0, 0
			if r.kind == rowGap {
				continue
			}
		}
		cols[len(cols)-1].rows = append(cols[len(cols)-1].rows, r)
		y += rh
		colW = max(colW, r.w)
	}
	// without the gaps left at their bottoms
	for i, c := range cols {
		if n := len(c.rows); n > 0 && c.rows[n-1].kind == rowGap {
			cols[i].rows = c.rows[:n-1]
		}
	}
	return cols, x + colW
}

// sectionH is the height of the section rows starts with, up to the next
// gap.
func sectionH(rows []summaryRow) int {
	h := 0
	for _, r := range rows {
		if r.kind == rowGap {
			break
		}
		h += panelLineH()
	}
	return h
}

// summaryLines is how many lines the summary needs to fit in width.
func (p *sysPanel) summaryLines(width, maxLines int) int {
	rows := p.summaryRows()
	for lines := panelMinLines; lines < maxLines; lines++ {
		if _, w := flowSummary(rows, lines); w <= width {
			return lines
		}
	}
	return maxLines
}

// fitHeight is the panel's height, for the summary to fit.
func (p *sysPanel) fitHeight() int {
	g := p.screen.Geometry()
	lines := p.summaryLines(int(g.W)-2*panelPad, panelMaxLines(int(g.H)))
	return panelChromeH() + lines*panelLineH()
}

func (p *sysPanel) drawSummary(a rect) {
	cols, _ := flowSummary(p.summaryRows(), a.h/panelLineH())
	for _, col := range cols {
		c := &column{p: p, x: a.x + col.x, y: a.y, w: 1 << 20, bottom: a.y + a.h}
		if c.x >= a.x+a.w {
			break
		}
		for _, r := range col.rows {
			if r.kind == rowGap {
				c.gap()
				continue
			}
			r.draw(c)
		}
	}
}

// rowWriter is where the sections put their rows: a column drawing them,
// or a rowCollector measuring them.
type rowWriter interface {
	header(s string)
	line(s string, fg uint32)
	meter(label string, pct float64, after string)
	coloredMeter(label string, pct float64, after string, color uint32)
}

func (p *sysPanel) memoryMeters(c rowWriter) {
	m := p.snap.mem
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
}

// filesystemMeters shows up to n filesystems' usage, and how many more.
func (p *sysPanel) filesystemMeters(c rowWriter, n int) {
	fs := p.snap.fs
	if len(fs) == 0 {
		c.line("none", colorDim)
	}
	for i, f := range fs {
		if i == n {
			c.line(fmt.Sprintf("%d more", len(fs)-n), colorDim)
			break
		}
		mount := f.mount
		if len(mount) > 9 {
			mount = "..." + mount[len(mount)-6:]
		}
		pct := percent(f.used, f.total)
		c.meter(fmt.Sprintf("%-9s", mount), pct, fmt.Sprintf("%9s of %s", humanize.IBytes(f.used), humanize.IBytes(f.total)))
	}
}

func (p *sysPanel) gpuLines(c rowWriter) {
	if len(p.snap.gpus) == 0 {
		c.line("no information on this system", colorDim)
	}
	for _, g := range p.snap.gpus {
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
}

func (p *sysPanel) drawCPU(a rect) {
	s := p.snap
	lh := panelLineH()
	c := &column{p: p, x: a.x, y: a.y, w: a.w, bottom: a.y + a.h}
	c.line(fmt.Sprintf("%s, %d threads   use %.0f%%   load %.2f %.2f %.2f", s.host.cpuModel, s.host.cores,
		s.cpuTotal, s.load[0], s.load[1], s.load[2]), colorText)

	// the whole CPU, and the load against the number of CPUs
	gh := max(2*lh, a.h*28/100)
	p.graph(rect{a.x, c.y, a.w*3/4 - panelPad, gh}, "cpu", 100, byLevel)
	p.label(a.x+4, c.y+2, "use %", colorText)
	lx := a.x + a.w*3/4
	loadTop := p.graph(rect{lx, c.y, a.w - a.w*3/4, gh}, "load", float64(max(1, s.host.cores)), flat(draculaPurple))
	p.label(lx+4, c.y+2, fmt.Sprintf("load, of %.0f", loadTop), colorText)
	y := c.y + gh + lh/2

	// the processes using the most, on the right when there is room
	if procW := 44 * charW(); a.w > 3*procW {
		pc := &column{p: p, x: a.x + a.w - procW, y: y, w: procW, bottom: a.y + a.h}
		pc.header("PROCESSES")
		if len(p.procs) == 0 {
			pc.line("measuring...", colorDim)
		}
		for _, u := range p.procs {
			fg := uint32(colorText)
			if u.cpu < 1 {
				fg = colorDim
			}
			pc.line(fmt.Sprintf("%7d %-18.18s %5.1f%% %8s", u.pid, u.name, u.cpu, humanize.IBytes(u.rss)), fg)
		}
		a.w -= procW + panelPad
	}

	// each core, in a grid
	n := len(s.cpus)
	if n == 0 {
		return
	}
	cols := max(1, a.w/(28*charW()))
	rows := (n + cols - 1) / cols
	cellW := a.w / cols
	cellH := max(2*lh, (a.y+a.h-y)/rows)
	for i, pct := range s.cpus {
		cx, cy := a.x+(i%cols)*cellW, y+(i/cols)*cellH
		if cy+cellH > a.y+a.h+1 {
			break
		}
		label := fmt.Sprintf("C%-3d %3.0f%%", i, pct)
		if t, ok := s.coreTemps[i]; ok {
			label += fmt.Sprintf("  %.0f\xb0C", t)
		}
		p.label(cx, cy, label, colorText)
		p.graph(rect{cx, cy + lh, cellW - panelPad, cellH - lh - 3}, cpuKey(i), 100, byLevel)
	}
}

func (p *sysPanel) drawMemory(a rect) {
	s := p.snap
	lh := panelLineH()
	gw := a.w * 60 / 100
	gh := a.h / 2
	if s.mem.swapTotal == 0 {
		gh = a.h - lh
	}
	p.label(a.x, a.y, fmt.Sprintf("used, of %s", humanize.IBytes(s.mem.total)), colorText)
	p.graph(rect{a.x, a.y + lh, gw, gh - lh - 2}, "mem", 100, byLevel)
	if s.mem.swapTotal > 0 {
		p.label(a.x, a.y+gh, fmt.Sprintf("swap, of %s", humanize.IBytes(s.mem.swapTotal)), colorText)
		p.graph(rect{a.x, a.y + gh + lh, gw, a.h - gh - lh}, "swap", 100, byLevel)
	}
	c := &column{p: p, x: a.x + gw + 2*panelPad, y: a.y, w: a.w - gw - 2*panelPad, bottom: a.y + a.h}
	c.header("MEMORY")
	p.memoryMeters(c)
	c.gap()
	c.line(fmt.Sprintf("total    %s", humanize.IBytes(s.mem.total)), colorText)
	if s.mem.swapTotal > 0 {
		c.line(fmt.Sprintf("swap     %s", humanize.IBytes(s.mem.swapTotal)), colorText)
	}
}

// ioGraphs draws, for each name, its rates in and out, graphed side by
// side, idle ones dimmed, as many as fit in a.
func (p *sysPanel) ioGraphs(a rect, names []string, rs []ioRate, totals map[string][2]uint64, prefix, in, out string, inColor, outColor uint32) {
	lh := panelLineH()
	rowH := 4 * lh
	fit := max(1, a.h/rowH)
	for i, name := range names {
		y := a.y + i*rowH
		if i == fit-1 && len(names) > fit {
			p.label(a.x, y, fmt.Sprintf("%d more", len(names)-i), colorDim)
			break
		}
		r, ok := rateOf(rs, name)
		t := totals[name]
		fg := uint32(colorText)
		status := fmt.Sprintf("%s %s  %s %s", in, formatRate(r.in), out, formatRate(r.out))
		if !ok {
			status, fg = "measuring...", colorDim
		} else if r.in+r.out == 0 {
			status, fg = "idle", colorDim
		}
		p.label(a.x, y, fmt.Sprintf("%-10s %-40s since boot: %s %s, %s %s", name, status,
			in, humanize.IBytes(t[0]), out, humanize.IBytes(t[1])), fg)
		gw := (a.w - panelPad) / 2
		g := rect{a.x, y + lh, gw, rowH - lh - lh/2}
		top := p.graph(g, prefix+name+":in", 0, flat(inColor))
		p.label(g.x+4, g.y+2, in+", peak "+formatRate(top), colorText)
		g.x += gw + panelPad
		top = p.graph(g, prefix+name+":out", 0, flat(outColor))
		p.label(g.x+4, g.y+2, out+", peak "+formatRate(top), colorText)
	}
}

func (p *sysPanel) drawDisk(a rect) {
	s := p.snap
	fw := a.w * 40 / 100
	c := &column{p: p, x: a.x, y: a.y, w: fw, bottom: a.y + a.h}
	c.header("FILESYSTEMS")
	fit := max(1, (c.bottom-c.y)/panelLineH()-1)
	for i, f := range s.fs {
		if i == fit {
			c.line(fmt.Sprintf("%d more", len(s.fs)-i), colorDim)
			break
		}
		pct := percent(f.used, f.total)
		c.meter(fmt.Sprintf("%-22.22s", f.mount), pct,
			fmt.Sprintf("%3.0f%%  %s of %s  %s", pct, humanize.IBytes(f.used), humanize.IBytes(f.total), f.fstype))
	}
	io := rect{a.x + fw + 2*panelPad, a.y, a.w - fw - 2*panelPad, a.h}
	(&column{p: p, x: io.x, y: io.y, w: io.w, bottom: io.y + io.h}).header("I/O")
	io.y += panelLineH()
	io.h -= panelLineH()
	p.ioGraphs(io, ioNames(s.diskTotals), s.disks, s.diskTotals, "disk:", "read", "write", draculaCyan, draculaPink)
}

func (p *sysPanel) drawNetwork(a rect) {
	s := p.snap
	p.ioGraphs(a, ioNames(s.netTotals), s.nets, s.netTotals, "net:", "in", "out", draculaGreen, draculaPurple)
}

func (p *sysPanel) drawSensors(a rect) {
	s := p.snap
	if len(s.allSensors) == 0 {
		p.label(a.x, a.y, "no sensors on this system", colorDim)
		return
	}
	lh := panelLineH()
	cols := max(1, a.w/(34*charW()))
	cellW := a.w / cols
	cellH := 3 * lh
	rows := max(1, a.h/cellH)
	for i, t := range s.allSensors {
		if i >= rows*cols {
			break
		}
		cx, cy := a.x+(i%cols)*cellW, a.y+(i/cols)*cellH
		p.label(cx, cy, fmt.Sprintf("%-22.22s %5.1f\xb0C", t.name, t.temp), colorText)
		p.graph(rect{cx, cy + lh, cellW - panelPad, cellH - lh - lh/2}, "temp:"+t.name, 100, byTemp)
	}
}

// drawMessages lists the last messages posted, the newest first.
func (p *sysPanel) drawMessages(a rect) {
	var history []*notification
	if nl := p.screen.wm.notes; nl != nil {
		history = nl.history
	}
	if len(history) == 0 {
		p.label(a.x, a.y, "no message yet: fion msg posts some, as fion msg -l error build failed", colorDim)
		return
	}
	c := &column{p: p, x: a.x, y: a.y, w: a.w, bottom: a.y + a.h}
	for i := len(history) - 1; i >= 0 && c.room(); i-- {
		n := history[i]
		c.text(c.x, fmt.Sprintf("%-5s", levelNames[n.level]), n.level.color(), true)
		c.text(c.x+6*charW(), n.at.Format("Jan 2 15:04:05"), colorDim, false)
		c.text(c.x+22*charW(), latin1(n.text), colorText, false)
		c.y += panelLineH()
	}
}

// drawPorts lists the TCP ports listened to, and by what, in columns.
func (p *sysPanel) drawPorts(a rect) {
	if !p.portsRead {
		p.label(a.x, a.y, "reading...", colorDim)
		return
	}
	if len(p.ports) == 0 {
		p.label(a.x, a.y, "no TCP port listened to, or none this system tells", colorDim)
		return
	}
	colW := 48 * charW()
	cols := max(1, (a.w+panelPad)/(colW+panelPad))
	rows := max(1, a.h/panelLineH()-1)
	for i := 0; i < cols; i++ {
		c := &column{p: p, x: a.x + i*(colW+panelPad), y: a.y, w: colW, bottom: a.y + a.h}
		if i*rows >= len(p.ports) {
			break
		}
		c.header(fmt.Sprintf("%-6s %-22s %7s %s", "PORT", "ADDRESS", "PID", "PROCESS"))
		for _, lp := range p.ports[i*rows : min(len(p.ports), (i+1)*rows)] {
			pid, name := "", lp.name
			if lp.pid > 0 {
				pid = fmt.Sprint(lp.pid)
			}
			if name == "" {
				name = "?"
			}
			fg := uint32(colorText)
			// those only this machine reaches
			if strings.HasPrefix(lp.addr, "127.") || lp.addr == "::1" || lp.addr == "localhost" {
				fg = colorDim
			}
			c.line(fmt.Sprintf("%-6d %-22.22s %7s %s", lp.port, lp.addr, pid, name), fg)
		}
	}
	if n := cols * rows; len(p.ports) > n {
		p.label(a.x, a.y+a.h-panelLineH(), fmt.Sprintf("%d more", len(p.ports)-n), colorDim)
	}
}

// panelKey switches the panel's views, or closes it.
func (wm *Manager) panelKey(ev xproto.KeyPressEvent) {
	km := wm.KeyboardManager
	p := wm.GetActiveScreen().panel
	sym := km.eventKeysym(ev.Detail, ev.State)
	mods := ev.State &^ (xproto.ModMaskLock | km.Num)
	n := len(panelViews)
	switch {
	case sym == XK_Escape, sym == XK_s && mods&^xproto.ModMaskShift == km.Mod:
		wm.GetActiveScreen().togglePanel()
		return
	case sym == XK_ISO_LeftTab, sym == XK_Tab && mods&xproto.ModMaskShift != 0, sym == XK_Left:
		p.view = (p.view + n - 1) % n
	case sym == XK_Tab, sym == XK_Right:
		p.view = (p.view + 1) % n
	case sym >= '1' && sym < xproto.Keysym('1'+n):
		p.view = int(sym - '1')
	default:
		return
	}
	p.gathered = time.Time{}
	p.gather()
	p.draw()
}

// panelClick selects the view whose name is under x, y, and reports
// whether there was one.
func (p *sysPanel) panelClick(x, y int) bool {
	if y > panelPad/2+panelLineH() {
		return false
	}
	for i, t := range p.tabs {
		if x >= t[0] && x < t[1] {
			p.view = i
			p.gathered = time.Time{}
			p.gather()
			p.draw()
			return true
		}
	}
	return false
}
