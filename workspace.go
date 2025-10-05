package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/BurntSushi/xgb/xproto"
	"github.com/dustin/go-humanize"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

type Workspace struct {
	Wm     *WM
	Screen *Screen

	Color uint32

	Ws      xproto.Window
	InfoBar xproto.Window

	RootFrame   *Frame
	ActiveFrame *Frame

	BarGC xproto.Gcontext // optional, for text
}

func newWorkspace(screen *Screen) (*Workspace, error) {
	wm := screen.wm
	geom := screen.Geom()

	w, err := xproto.NewWindowId(wm.X)
	if err != nil {
		return nil, err
	}

	ws := &Workspace{
		Wm:     screen.wm,
		Screen: screen,
		Color:  wm.randomColor(),
		Ws:     w,
	}

	xproto.CreateWindow(
		wm.X, wm.Scr.RootDepth, w, wm.Root,
		geom.X, geom.Y,
		geom.W, geom.H, // Adjust height for top and bottom borders
		0, // Set border width to 1px
		xproto.WindowClassInputOutput, wm.Scr.RootVisual,
		xproto.CwBackPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			ws.Color,
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)

	ws.setupLayout()

	return ws, nil
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
	geom := ws.Screen.Geom()

	w, err := xproto.NewWindowId(wm.X)
	if err != nil {
		return err
	}

	xproto.CreateWindow(
		wm.X, wm.Scr.RootDepth, w, ws.Ws,
		geom.X, int16(ws.Screen.ScreenInfo.HeightInPixels)-20, // Position at the bottom
		geom.W-(2), 20-2, // Adjust height for top and bottom borders
		1, // Set border width to 1px
		xproto.WindowClassInputOutput, wm.Scr.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			ws.Screen.ScreenInfo.BlackPixel,
			ws.Color, // Set the border color
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)
	ws.InfoBar = w

	// Create a GC and set a core font
	gc, _ := xproto.NewGcontextId(wm.X)
	xproto.CreateGC(wm.X, gc, xproto.Drawable(ws.InfoBar),
		xproto.GcForeground|xproto.GcBackground, []uint32{
			ws.Screen.ScreenInfo.WhitePixel, // text color
			ws.Screen.ScreenInfo.BlackPixel, // bg (unused by ImageText8)
		},
	)
	// Load a core font and bind it to the GC
	fid, _ := xproto.NewFontId(wm.X)
	_ = xproto.OpenFontChecked(wm.X, fid, uint16(len("fixed")), "fixed").Check()
	xproto.ChangeGC(wm.X, gc, xproto.GcFont, []uint32{uint32(fid)})
	ws.BarGC = gc

	return nil
}

func (ws *Workspace) setupRootFrame() error {
	if frame, err := newFrame(ws); err != nil {
		return err
	} else {
		ws.RootFrame = frame
		ws.ActiveFrame = ws.RootFrame
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
					xproto.MapWindow(ws.Wm.X, cl.Win)
					xproto.MapWindow(ws.Wm.X, cl.Frame)
				}
			}
		})
	*/
	xproto.MapWindow(ws.Wm.X, ws.InfoBar)
	ws.RootFrame.Map()
	xproto.MapWindow(ws.Wm.X, ws.Ws)
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
					xproto.UnmapWindow(ws.Wm.X, cl.Win)
					xproto.UnmapWindow(ws.Wm.X, cl.Frame)
				}
			}
		})
	*/
	ws.RootFrame.Unmap()
	xproto.UnmapWindow(ws.Wm.X, ws.InfoBar)
	xproto.UnmapWindow(ws.Wm.X, ws.Ws)
}

func (ws *Workspace) Destroy() {
	/*
		walk(ws.Root, func(n *FrameNode) {
			if n.Kind != Leaf {
				return
			}
			for _, w := range n.Tabs {
				if cl := ws.Wm.Clients[w]; cl != nil {
					xproto.DestroyWindow(ws.Wm.X, cl.Win)
					xproto.DestroyWindow(ws.Wm.X, cl.Frame)
				}
			}
		})
	*/
	xproto.DestroyWindow(ws.Wm.X, ws.Ws)
}

func (ws *Workspace) updateInfoBar() {
	var wsOffset int
	var screenOffset int
	for offset, currScreen := range ws.Wm.Screens {
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

	txt := fmt.Sprintf("[%d:%d]", screenOffset, wsOffset)
	// Clear the bar area (optional)
	xproto.PolyFillRectangle(ws.Wm.X, xproto.Drawable(ws.InfoBar), ws.BarGC,
		[]xproto.Rectangle{{X: 0, Y: 0, Width: 0, Height: 20}})
	xproto.ImageText8(ws.Wm.X, byte(len(txt)), xproto.Drawable(ws.InfoBar), ws.BarGC, 5, 13, txt)
	xproto.ImageText8(ws.Wm.X, byte(len(clock)), xproto.Drawable(ws.InfoBar), ws.BarGC, int16(ws.Screen.Geom().W)-200, 13, clock)

	// get the CPU and memory usage
	ressources := []string{}
	cpuPercents, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercents) > 0 {
		ressources = append(ressources, fmt.Sprintf("CPU: %.02f%%", cpuPercents[0]))
	}

	memPercents, err := mem.VirtualMemory()
	if err == nil {
		ressources = append(ressources, fmt.Sprintf("Mem: %s/%s",
			humanize.IBytes(memPercents.Used), humanize.IBytes(memPercents.Total)))
	}

	ressources = append(ressources, fmt.Sprintf("active frame: %p", ws.ActiveFrame))

	ressourcesStr := strings.Join(ressources, " | ")
	xproto.ImageText8(ws.Wm.X, byte(len(ressourcesStr)), xproto.Drawable(ws.InfoBar), ws.BarGC, 40, 13, ressourcesStr)
}

func (ws *Workspace) GetActiveFrame() *Frame {
	return ws.ActiveFrame
}
