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
		geom.X, int16(int(ws.Screen.Info().HeightInPixels)-infoBarOuterH()), // at the bottom
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
	if fid, ok := openFont(ws.Conn(), font.plain); ok {
		xproto.ChangeGC(ws.Conn(), gc, xproto.GcFont, []uint32{uint32(fid)})
		xproto.CloseFont(ws.Conn(), fid)
	}
	ws.InfoBarGC = gc

	// the same font in bold, for values past their threshold
	if bid, ok := openFont(ws.Conn(), font.bold); ok {
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

// the info bar's height, inside its border, and with it
func infoBarInnerH() int { return textH() + 5 }
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

func (ws *Workspace) updateInfoBar() {
	clock := time.Now().Format(time.RFC1123)

	// CPU, memory and load, alerts when past their thresholds
	var resources []barText
	cores, _ := cpu.Counts(true)
	cpuPercent, memPercent, load1 := -1.0, -1.0, -1.0
	if p, err := cpu.Percent(0, false); err == nil && len(p) > 0 {
		cpuPercent = p[0]
	}
	vm, err := mem.VirtualMemory()
	if err == nil {
		memPercent = vm.UsedPercent
	}
	avg, loadErr := load.Avg()
	if loadErr == nil {
		load1 = avg.Load1
	}
	cpuAlert, memAlert, loadAlert := barAlerts(cpuPercent, memPercent, load1, cores)
	if cpuPercent >= 0 {
		resources = append(resources,
			barText{s: fmt.Sprintf("CPU: % 4.02f%%", cpuPercent), alert: cpuAlert},
			barText{s: fmt.Sprintf(" (%d cores) | ", cores)})
	}
	if memPercent >= 0 {
		resources = append(resources, barText{s: fmt.Sprintf("MEM: % 4s / % 4s (%.02f%%)",
			humanize.IBytes(vm.Used), humanize.IBytes(vm.Total), vm.UsedPercent), alert: memAlert},
			barText{s: " | "})
	}
	if loadErr == nil {
		resources = append(resources, barText{s: fmt.Sprintf("LOAD: %.2f %.2f %.2f", avg.Load1, avg.Load5, avg.Load15),
			alert: loadAlert})
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
				xproto.PolyText8(conn, bar, gc, int16(x+1), int16(baseline(infoBarInnerH())), append([]byte{byte(len(s)), 0}, s...))
			}
		}
		xproto.ImageText8(conn, byte(len(s)), bar, gc, int16(x), int16(baseline(infoBarInnerH())), s)
		if t.alert && ws.infoBarAlertGC == 0 {
			xproto.ChangeGC(conn, ws.InfoBarGC, xproto.GcForeground, []uint32{colorText})
		}
		return x + len(s)*charW()
	}
	image := func(x int, img logoImage) int {
		xproto.CopyArea(conn, xproto.Drawable(img.pixmap), bar, ws.Screen.logoGC,
			0, 0, int16(x), int16((infoBarInnerH()-img.h)/2), uint16(img.w), uint16(img.h))
		return x + img.w
	}

	// where we are, then the operating system
	x := 4
	screen, workspace, count := ws.position()
	x = text(x, barText{s: fmt.Sprintf("[%02x:%02x/%02x] | ", screen, workspace, count)})
	system := thisOS()
	if img, ok := ws.Screen.iconPixmap(system.kind, colorBar); ok {
		x = image(x, img) + 4
	}
	x = text(x, barText{s: system.String() + " | "})
	for _, t := range resources {
		x = text(x, t)
	}
	if b, ok := readBattery(); ok {
		x = text(x, barText{s: " | "})
		text(x, barText{s: formatBattery(b), alert: batteryAlert(b)})
	}
	text(int(ws.Screen.Geometry().W)-(len(clock)+1)*charW(), barText{s: clock})
}

func (ws *Workspace) updateTitleBars() {
	walk(ws.Root, func(f *Frame) {
		f.updateTitleBar()
	})
}

func (ws *Workspace) GetActiveFrame() *Frame {
	return ws.ActiveFrame
}
