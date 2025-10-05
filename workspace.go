package main

import (
	"github.com/BurntSushi/xgb/xproto"
)

type Workspace struct {
	Wm     *WM
	Screen *Screen

	Root         *FrameNode
	FocusedFrame *FrameNode

	Color       uint32
	BorderWidth uint16

	Ws   xproto.Window
	Bar  xproto.Window
	Area xproto.Window

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
		Wm:           screen.wm,
		Screen:       screen,
		Root:         &FrameNode{Kind: Leaf, G: screen.Geom(), Active: -1},
		FocusedFrame: nil,
		Color:        wm.randomColor(),
		BorderWidth:  1,
		Ws:           w,
	}
	ws.FocusedFrame = ws.Root

	xproto.CreateWindow(
		wm.X, wm.Scr.RootDepth, w, wm.Root,
		geom.X, geom.Y,
		geom.W-(2*ws.BorderWidth), geom.H-(2*ws.BorderWidth), // Adjust height for top and bottom borders
		ws.BorderWidth, // Set border width to 1px
		xproto.WindowClassInputOutput, wm.Scr.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			screen.ScreenInfo.BlackPixel,
			ws.Color, // Set the border color
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
	if err := ws.setupWorkArea(); err != nil {
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
		geom.W, 20, // Adjust height for top and bottom borders
		ws.BorderWidth, // Set border width to 1px
		xproto.WindowClassInputOutput, wm.Scr.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			ws.Screen.ScreenInfo.BlackPixel,
			ws.Color, // Set the border color
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)
	ws.Bar = w

	// Create a GC and set a core font
	gc, _ := xproto.NewGcontextId(wm.X)
	xproto.CreateGC(wm.X, gc, xproto.Drawable(ws.Bar),
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

func (ws *Workspace) setupWorkArea() error {
	wm := ws.Screen.wm
	geom := ws.Screen.Geom()

	w, err := xproto.NewWindowId(wm.X)
	if err != nil {
		return err
	}

	xproto.CreateWindow(
		wm.X, wm.Scr.RootDepth, w, ws.Ws,
		geom.X, geom.Y, // Position at the bottom
		geom.W-(2*ws.BorderWidth), uint16(ws.Screen.ScreenInfo.HeightInPixels)-20-(2*ws.BorderWidth), // Adjust height for top and bottom borders
		ws.BorderWidth, // Set border width to 1px
		xproto.WindowClassInputOutput, wm.Scr.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			ws.Screen.ScreenInfo.BlackPixel,
			ws.Color, // Set the border color
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)
	ws.Area = w

	return nil
}

func (ws *Workspace) Map() {
	// Unmap all leaf clients and frames
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
	xproto.MapWindow(ws.Wm.X, ws.Bar)
	xproto.MapWindow(ws.Wm.X, ws.Area)
	xproto.MapWindow(ws.Wm.X, ws.Ws)
}

func (ws *Workspace) Unmap() {
	// Unmap all leaf clients and frames
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
	xproto.UnmapWindow(ws.Wm.X, ws.Area)
	xproto.UnmapWindow(ws.Wm.X, ws.Bar)
	xproto.UnmapWindow(ws.Wm.X, ws.Ws)
}

func (ws *Workspace) Destroy() {
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
	xproto.DestroyWindow(ws.Wm.X, ws.Ws)
}
