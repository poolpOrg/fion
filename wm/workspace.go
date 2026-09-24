package wm

import (
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

type Workspace struct {
	Manager *Manager
	Screen  *Screen

	Color           uint32
	WorkspaceWindow xproto.Window
	InfoBarWindow   xproto.Window
	InfoBarGC       xproto.Gcontext

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
		Color:           0x424242,
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
			ws.Color,
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
		geom.X, int16(ws.Screen.Info().HeightInPixels)-20, // Position at the bottom
		geom.W-(2), 20-2, // Adjust height for top and bottom borders
		1, // Set border width to 1px
		xproto.WindowClassInputOutput, ws.Screen.Info().RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			ws.Screen.Info().BlackPixel,
			ws.Color, // Set the border color
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)
	ws.InfoBarWindow = w

	// Create a GC and set a core font
	gc, _ := xproto.NewGcontextId(ws.Conn())
	xproto.CreateGC(ws.Conn(), gc, xproto.Drawable(ws.InfoBarWindow),
		xproto.GcForeground|xproto.GcBackground, []uint32{
			ws.Screen.Info().WhitePixel, // text color
			ws.Screen.Info().BlackPixel, // bg (unused by ImageText8)
		},
	)
	// Load a core font and bind it to the GC
	fid, _ := xproto.NewFontId(ws.Conn())
	_ = xproto.OpenFontChecked(ws.Conn(), fid, uint16(len("fixed")), "fixed").Check()
	xproto.ChangeGC(ws.Conn(), gc, xproto.GcFont, []uint32{uint32(fid)})
	ws.InfoBarGC = gc

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

func (ws *Workspace) updateInfoBar() {
	var wsOffset int
	var screenOffset int
	for offset, currScreen := range ws.Manager.Screens {
		if ws.Screen == currScreen {
			screenOffset = offset
			break
		}
	}

	for offset, currWs := range ws.Screen.Workspaces {
		if currWs == ws {
			wsOffset = offset
			break
		}
	}

	clock := time.Now().Format(time.RFC1123)

	xproto.PolyFillRectangle(ws.Manager.Conn(), xproto.Drawable(ws.InfoBarWindow), ws.InfoBarGC,
		[]xproto.Rectangle{{X: 0, Y: 0, Width: 0, Height: 20}})

	// get the CPU and memory usage
	ressources := []string{}
	numCpus, _ := cpu.Counts(true)

	cpuPercents, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercents) > 0 {
		ressources = append(ressources, fmt.Sprintf("CPU: % 4.02f%% (%d cores)", cpuPercents[0], numCpus))
	}

	memPercents, err := mem.VirtualMemory()
	if err == nil {
		ressources = append(ressources, fmt.Sprintf("MEM: % 4s / % 4s (%.02f%%)",
			humanize.IBytes(memPercents.Used), humanize.IBytes(memPercents.Total),
			memPercents.UsedPercent))
	}

	infotext := fmt.Sprintf("FION | [%02x:%02x/%02x] | ", screenOffset+1, wsOffset+1, len(ws.Screen.Workspaces)-1) + strings.Join(ressources, " | ")
	xproto.ClearArea(ws.Manager.Conn(), false, ws.InfoBarWindow, 0, 0, 0, 0)
	xproto.ImageText8(ws.Manager.Conn(), byte(len(infotext)), xproto.Drawable(ws.InfoBarWindow), ws.InfoBarGC, 5, 14, infotext)
	xproto.ImageText8(ws.Manager.Conn(), byte(len(clock)), xproto.Drawable(ws.InfoBarWindow), ws.InfoBarGC, int16(ws.Screen.Geometry().W)-190, 14, clock)

}

func (ws *Workspace) updateTitleBars() {
	walk(ws.Root, func(f *Frame) {
		f.updateTitleBar()
	})
}

func (ws *Workspace) GetActiveFrame() *Frame {
	return ws.ActiveFrame
}

func (ws *Workspace) cycleFrameLeft() {
	if ws.Root.Leaf() {
		return
	}

	frames := []*Frame{}
	i := 0
	walk(ws.Root, func(f *Frame) {
		if !f.Leaf() {
			return
		}
		if f == ws.ActiveFrame {
			i = len(frames)
		}
		frames = append(frames, f)
	})

	i = (i + len(frames) - 1) % len(frames)
	ws.ActiveFrame = frames[i]
}

func (ws *Workspace) cycleFrameRight() {
	if ws.Root.Leaf() {
		return
	}

	frames := []*Frame{}
	i := 0
	walk(ws.Root, func(f *Frame) {
		if !f.Leaf() {
			return
		}
		if f == ws.ActiveFrame {
			i = len(frames)
		}
		frames = append(frames, f)
	})

	i = (i + 1) % len(frames)
	ws.ActiveFrame = frames[i]
}
