package wm

import (
	"fmt"
	"slices"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

type Workspace struct {
	Manager *Manager
	Screen  *Screen

	WorkspaceWindow xproto.Window
	InfoBarWindow   xproto.Window
	InfoBarGC       xproto.Gcontext
	infoBarAlertGC  xproto.Gcontext // bold red, 0 when the bold font is missing

	Root        *Frame
	ActiveFrame *Frame

	// the project it was opened for, shown in the bar, "" when none
	name string
}

func newWorkspace(screen *Screen) (*Workspace, error) {
	wm := screen.wm
	geom := screen.Geometry()

	w, err := xproto.NewWindowId(wm.Conn())
	if err != nil {
		return nil, err
	}

	ws := &Workspace{
		Manager:         screen.wm,
		Screen:          screen,
		WorkspaceWindow: w,
	}

	xproto.CreateWindow(
		wm.Conn(), screen.Info().RootDepth, w, screen.Info().Root,
		geom.X, geom.Y,
		geom.W, geom.H, // Adjust height for top and bottom borders
		0, // Set border width to 1px
		xproto.WindowClassInputOutput, screen.Info().RootVisual,
		xproto.CwBackPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			colorBackground,
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)

	ws.setupLayout()

	return ws, nil
}

func (ws *Workspace) Conn() *xgb.Conn {
	return ws.Manager.Conn()
}

func (ws *Workspace) setupLayout() error {
	if err := ws.setupInfoBar(); err != nil {
		return err
	}
	if err := ws.setupRootFrame(); err != nil {
		return err
	}
	return nil
}

func (ws *Workspace) setupInfoBar() error {
	wm := ws.Screen.wm
	geom := ws.Screen.Geometry()

	w, err := xproto.NewWindowId(wm.Conn())
	if err != nil {
		return err
	}

	xproto.CreateWindow(
		ws.Conn(), ws.Screen.Info().RootDepth, w, ws.WorkspaceWindow,
		0, int16(int(geom.H)-infoBarOuterH()), // at the bottom
		geom.W-(2), uint16(infoBarInnerH()), // inside its border
		1, // Set border width to 1px
		xproto.WindowClassInputOutput, ws.Screen.Info().RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			colorBar,
			colorBorder,
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)
	ws.InfoBarWindow = w

	// Create a GC and set a core font
	gc, _ := xproto.NewGcontextId(ws.Conn())
	xproto.CreateGC(ws.Conn(), gc, xproto.Drawable(ws.InfoBarWindow),
		xproto.GcForeground|xproto.GcBackground, []uint32{
			colorText, // text color
			colorBar,  // background, of the text drawn by ImageText8
		},
	)
	if fid, ok := openFont(ws.Conn(), barFont.plain); ok {
		xproto.ChangeGC(ws.Conn(), gc, xproto.GcFont, []uint32{uint32(fid)})
		xproto.CloseFont(ws.Conn(), fid)
	}
	ws.InfoBarGC = gc

	// the same font in bold, for values past their threshold
	if bid, ok := openFont(ws.Conn(), barFont.bold); ok {
		agc, _ := xproto.NewGcontextId(ws.Conn())
		xproto.CreateGC(ws.Conn(), agc, xproto.Drawable(ws.InfoBarWindow),
			xproto.GcForeground|xproto.GcBackground|xproto.GcFont,
			[]uint32{colorAlert, colorBar, uint32(bid)})
		xproto.CloseFont(ws.Conn(), bid)
		ws.infoBarAlertGC = agc
	}

	return nil
}

func (ws *Workspace) setupRootFrame() error {
	if frame, err := newRootFrame(ws); err != nil {
		return err
	} else {
		ws.Root = frame
		ws.ActiveFrame = ws.Root
		return nil
	}
}

func (ws *Workspace) splitH() error {
	activeFrame := ws.GetActiveFrame()
	if activeFrame == nil {
		return fmt.Errorf("no active frame to split")
	}
	return activeFrame.splitH()
}

func (ws *Workspace) splitV() error {
	activeFrame := ws.GetActiveFrame()
	if activeFrame == nil {
		return fmt.Errorf("no active frame to split")
	}
	return activeFrame.splitV()
}

func (ws *Workspace) Map() {
	// Unmap all leaf clients and frames
	/*
		walk(ws.Root, func(n *FrameNode) {
			if n.Kind != Leaf {
				return
			}
			for _, w := range n.Tabs {
				if cl := ws.Wm.Clients[w]; cl != nil {
					xproto.MapWindow(ws.ws.Conn(), cl.Win)
					xproto.MapWindow(ws.ws.Conn(), cl.Frame)
				}
			}
		})
	*/
	if ws.InfoBarWindow != 0 {
		xproto.MapWindow(ws.Manager.Conn(), ws.InfoBarWindow)
	}
	ws.Root.Map()
	xproto.MapWindow(ws.Manager.Conn(), ws.WorkspaceWindow)
	ws.Manager.showDialogs(ws, true)
	ws.Screen.raiseScratchpad()
}

func (ws *Workspace) Unmap() {
	// Unmap all leaf clients and frames
	/*
		walk(ws.Root, func(n *FrameNode) {
			if n.Kind != Leaf {
				return
			}
			for _, w := range n.Tabs {
				if cl := ws.Wm.Clients[w]; cl != nil {
					xproto.UnmapWindow(ws.ws.Conn(), cl.Win)
					xproto.UnmapWindow(ws.ws.Conn(), cl.Frame)
				}
			}
		})
	*/
	ws.Root.Unmap()
	if ws.InfoBarWindow != 0 {
		xproto.UnmapWindow(ws.Manager.Conn(), ws.InfoBarWindow)
	}
	xproto.UnmapWindow(ws.Manager.Conn(), ws.WorkspaceWindow)
	ws.Manager.showDialogs(ws, false)
}

func (ws *Workspace) Destroy() {
	/*
		walk(ws.Root, func(n *FrameNode) {
			if n.Kind != Leaf {
				return
			}
			for _, w := range n.Tabs {
				if cl := ws.Wm.Clients[w]; cl != nil {
					xproto.DestroyWindow(ws.ws.Conn(), cl.Win)
					xproto.DestroyWindow(ws.ws.Conn(), cl.Frame)
				}
			}
		})
	*/
	xproto.DestroyWindow(ws.Manager.Conn(), ws.WorkspaceWindow)
}

// position returns the numbers, from 1, of the workspace's screen and of the
// workspace on it, and how many workspaces the screen has.
func (ws *Workspace) position() (screen, workspace, count int) {
	screen = slices.Index(ws.Manager.Screens, ws.Screen) + 1
	workspace = slices.Index(ws.Screen.Workspaces, ws) + 1
	return screen, workspace, len(ws.Screen.Workspaces)
}

// barBaseline is where the bar's text sits.
func barBaseline() int {
	return (infoBarInnerH()-barFont.ascent-barFont.descent)/2 + barFont.ascent
}

// the info bar's height, inside its border, and with it
func infoBarInnerH() int { return barFont.ascent + barFont.descent + 7 }
func infoBarOuterH() int { return infoBarInnerH() + 2 }

// barAlerts tells which of the CPU, memory and load values are past their
// thresholds, to show in bold red.
func barAlerts(cpuPercent, memPercent, load1 float64, cores int) (cpuAlert, memAlert, loadAlert bool) {
	return cpuPercent >= alertCPUPercent,
		memPercent >= alertMemPercent,
		cores > 0 && load1 > float64(cores)
}

// a piece of the info bar's text
type barText struct {
	s     string
	alert bool
}

// barState is what the info bar shows, sampled once per update.
type barState struct {
	now                    time.Time
	cores                  int
	cpuPercent, memPercent float64 // -1 when unknown
	memUsed, memTotal      uint64
	load                   [3]float64
	hasLoad                bool
	battery                *batteryInfo
	recording              time.Duration // -1 when not recording
	position               string
	urgent                 string // the workspaces asking for attention
	system                 string
}

// the bar's forms, from the most detailed to the most compact, used by
// how much fits the screen's width
const barForms = 4

// barPieces is the bar's text in form 0 to barForms-1: before the
// operating system's icon, after it, and the clock on the right.
func barPieces(st barState, form int) (before, after []barText, clock string) {
	if st.recording >= 0 {
		d := st.recording.Round(time.Second)
		before = append(before, barText{s: fmt.Sprintf("REC %d:%02d", int(d.Minutes()), int(d.Seconds())%60), alert: true},
			barText{s: " | "})
	}
	before = append(before, barText{s: st.position + " | "})
	if st.urgent != "" {
		before = append(before, barText{s: st.urgent, alert: true}, barText{s: " | "})
	}

	if form < 3 {
		after = append(after, barText{s: st.system + " | "})
	}
	cpuAlert, memAlert, loadAlert := barAlerts(st.cpuPercent, st.memPercent, st.load[0], st.cores)
	if st.cpuPercent >= 0 {
		switch {
		case form == 0:
			after = append(after, barText{s: fmt.Sprintf("CPU: % 4.02f%%", st.cpuPercent), alert: cpuAlert},
				barText{s: fmt.Sprintf(" (%d cores) | ", st.cores)})
		case form == 1:
			after = append(after, barText{s: fmt.Sprintf("CPU: % 4.02f%%", st.cpuPercent), alert: cpuAlert}, barText{s: " | "})
		default:
			after = append(after, barText{s: fmt.Sprintf("CPU: %.0f%%", st.cpuPercent), alert: cpuAlert}, barText{s: " | "})
		}
	}
	if st.memPercent >= 0 {
		m := fmt.Sprintf("MEM: % 4s / % 4s (%.02f%%)", humanize.IBytes(st.memUsed), humanize.IBytes(st.memTotal), st.memPercent)
		if form >= 2 {
			m = fmt.Sprintf("MEM: %.0f%%", st.memPercent)
		}
		after = append(after, barText{s: m, alert: memAlert}, barText{s: " | "})
	}
	if st.hasLoad {
		l := fmt.Sprintf("LOAD: %.2f %.2f %.2f", st.load[0], st.load[1], st.load[2])
		if form >= 2 {
			l = fmt.Sprintf("LOAD: %.2f", st.load[0])
		}
		after = append(after, barText{s: l, alert: loadAlert})
	}
	if st.battery != nil {
		after = append(after, barText{s: " | "}, barText{s: formatBattery(*st.battery), alert: batteryAlert(*st.battery)})
	}
	clock = st.now.Format(time.RFC1123)
	if form >= 1 {
		clock = st.now.Format("Mon 2 Jan 15:04")
	}
	return before, after, clock
}

func (ws *Workspace) sampleBar() barState {
	st := barState{now: time.Now(), cpuPercent: -1, memPercent: -1, recording: -1}
	st.cores, _ = cpu.Counts(true)
	if p, err := cpu.Percent(0, false); err == nil && len(p) > 0 {
		st.cpuPercent = p[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		st.memPercent, st.memUsed, st.memTotal = vm.UsedPercent, vm.Used, vm.Total
	}
	if avg, err := load.Avg(); err == nil {
		st.load, st.hasLoad = [3]float64{avg.Load1, avg.Load5, avg.Load15}, true
	}
	if b, ok := readBattery(); ok {
		st.battery = &b
	}
	if rec := ws.Manager.recording; rec != nil {
		st.recording = time.Since(rec.started)
	}
	screen, workspace, count := ws.position()
	st.position = fmt.Sprintf("[%02x:%02x/%02x]", screen, workspace, count)
	if ws.name != "" {
		st.position = fmt.Sprintf("[%02x:%02x/%02x %s]", screen, workspace, count, ws.name)
	}
	st.urgent = ws.Screen.urgentSummary()
	st.system = thisOS().String()
	return st
}

func textWidth(ts []barText) int {
	n := 0
	for _, t := range ts {
		n += len(t.s)
	}
	return n * barFont.charW
}

func (ws *Workspace) updateInfoBar() {
	st := ws.sampleBar()
	width := int(ws.Screen.Geometry().W)
	system := thisOS()
	icon, hasIcon := ws.Screen.iconPixmap(system.kind, colorBar)
	iconW := 0
	if hasIcon {
		iconW = icon.w + 4
	}

	// the most detailed form that fits, a space before the clock
	var before, after []barText
	var clock string
	for form := range barForms {
		before, after, clock = barPieces(st, form)
		if 4+textWidth(before)+iconW+textWidth(after)+(len(clock)+3)*barFont.charW <= width {
			break
		}
	}

	conn, bar := ws.Manager.Conn(), xproto.Drawable(ws.InfoBarWindow)
	xproto.ClearArea(conn, false, ws.InfoBarWindow, 0, 0, 0, 0)
	text := func(x int, t barText) int {
		s := t.s
		if len(s) > 255 {
			s = s[:255]
		}
		gc := ws.InfoBarGC
		if t.alert {
			if ws.infoBarAlertGC != 0 {
				gc = ws.infoBarAlertGC
			} else {
				// no bold font: red, drawn twice for weight
				xproto.ChangeGC(conn, ws.InfoBarGC, xproto.GcForeground, []uint32{colorAlert})
				xproto.PolyText8(conn, bar, gc, int16(x+1), int16(barBaseline()), append([]byte{byte(len(s)), 0}, s...))
			}
		}
		xproto.ImageText8(conn, byte(len(s)), bar, gc, int16(x), int16(barBaseline()), s)
		if t.alert && ws.infoBarAlertGC == 0 {
			xproto.ChangeGC(conn, ws.InfoBarGC, xproto.GcForeground, []uint32{colorText})
		}
		return x + len(s)*barFont.charW
	}

	x := 4
	for _, t := range before {
		x = text(x, t)
	}
	if hasIcon {
		xproto.CopyArea(conn, xproto.Drawable(icon.pixmap), bar, ws.Screen.logoGC,
			0, 0, int16(x), int16((infoBarInnerH()-icon.h)/2), uint16(icon.w), uint16(icon.h))
		x += iconW
	}
	for _, t := range after {
		x = text(x, t)
	}
	text(width-(len(clock)+1)*barFont.charW, barText{s: clock})
}

func (ws *Workspace) updateTitleBars() {
	walk(ws.Root, func(f *Frame) {
		f.updateTitleBar()
	})
}

func (ws *Workspace) GetActiveFrame() *Frame {
	return ws.ActiveFrame
}
