package main

import (
	"fmt"

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
	// BarBgPixel uint32
	// BarFgPixel uint32
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

	xproto.MapWindow(wm.X, w)

	return ws, nil
}

func (ws *Workspace) String() string {
	return fmt.Sprintf("Workspace{Root:%p, FocusedFrame:%p}", ws.Root, ws.FocusedFrame)
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
}

func (ws *Workspace) Visible() {
	ws.Wm.ActiveWorkspace().Unmap()
	ws.Wm.ActiveWorkspace().FocusedFrame = nil
	ws.Wm.ActiveWorkspace().FocusedFrame = ws.FocusedFrame
	ws.Map()
	ws.Wm.layoutWorkspace(ws)
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
