package main

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

type Frame struct {
	workspace *Workspace
	window    xproto.Window

	parent       *Frame
	children     []*Frame        // for SplitH/SplitV
	clients      []xproto.Window // for Leaf: client windows (tab order)
	activeClient int             // index into Tabs

	g        Rect // assigned geometry during layout
	titleBar xproto.Window
	barGC    xproto.Gcontext

	leaf bool
}

func newRootFrame(ws *Workspace) (*Frame, error) {
	geom := ws.Screen.Geometry()

	w, err := xproto.NewWindowId(ws.Conn())
	if err != nil {
		return nil, err
	}

	frameGeom := Rect{
		X: 0,
		Y: 0,
		W: geom.W,
		H: uint16(ws.Screen.ScreenInfo.HeightInPixels) - 20,
	}

	f := &Frame{
		workspace:    ws,
		window:       w,
		parent:       nil,
		activeClient: -1,
		g:            frameGeom,
		leaf:         true,
	}

	xproto.CreateWindow(
		f.Conn(), ws.Screen.ScreenInfo.RootDepth, w, ws.WorkspaceWindow,
		frameGeom.X, frameGeom.Y, // Position at the bottom
		frameGeom.W, frameGeom.H, // Adjust height for top and bottom borders
		0, // Set border width to 1px
		xproto.WindowClassInputOutput, ws.Screen.ScreenInfo.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			ws.Screen.ScreenInfo.BlackPixel,
			ws.Color, // Set the border color
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)

	f.setuptitleBar()

	return f, nil
}

func (f *Frame) Conn() *xgb.Conn {
	return f.workspace.Manager.Conn()
}

func (f *Frame) GetWindow() xproto.Window {
	return f.window
}

func (f *Frame) GetParent() *Frame {
	return f.parent
}

func (f *Frame) Leaf() bool {
	return f.leaf
}

func (f *Frame) setuptitleBar() error {
	ws := f.workspace
	geom := f.g

	w, err := xproto.NewWindowId(f.Conn())
	if err != nil {
		return err
	}

	xproto.CreateWindow(
		f.Conn(), ws.Screen.ScreenInfo.RootDepth, w, f.window,
		0, 0, // Position at the bottom
		geom.W-2, 20, // Adjust height for top and bottom borders
		1, // Set border width to 1px
		xproto.WindowClassInputOutput, ws.Screen.ScreenInfo.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			f.workspace.Screen.ScreenInfo.BlackPixel,
			f.workspace.Color, // Set the border color
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)
	f.titleBar = w

	// Create a GC and set a core font
	gc, _ := xproto.NewGcontextId(f.Conn())
	xproto.CreateGC(f.Conn(), gc, xproto.Drawable(f.titleBar),
		xproto.GcForeground|xproto.GcBackground, []uint32{
			f.workspace.Screen.ScreenInfo.WhitePixel, // text color
			f.workspace.Screen.ScreenInfo.BlackPixel, // bg (unused by ImageText8)
		},
	)
	// Load a core font and bind it to the GC
	fid, _ := xproto.NewFontId(f.Conn())
	_ = xproto.OpenFontChecked(f.Conn(), fid, uint16(len("fixed")), "fixed").Check()
	xproto.ChangeGC(f.Conn(), gc, xproto.GcFont, []uint32{uint32(fid)})
	f.barGC = gc

	f.updateTitleBar()
	xproto.MapWindow(f.Conn(), f.titleBar)

	return nil
}

func (f *Frame) Map() {
	for _, child := range f.children {
		child.Map()
	}
	xproto.MapWindow(f.Conn(), f.titleBar)
	xproto.MapWindow(f.Conn(), f.window)
}

func (f *Frame) Unmap() {
	xproto.UnmapWindow(f.Conn(), f.titleBar)
	xproto.UnmapWindow(f.Conn(), f.window)
	for _, child := range f.children {
		child.Unmap()
	}
}

func (f *Frame) Destroy() {
	xproto.DestroyWindow(f.Conn(), f.window)
}

func (f *Frame) split(geom1, geom2 Rect) error {
	ws := f.workspace

	f1W, err := xproto.NewWindowId(f.Conn())
	if err != nil {
		return err
	}

	f2W, err := xproto.NewWindowId(f.Conn())
	if err != nil {
		return err
	}

	f1Frame := &Frame{
		workspace:    f.workspace,
		window:       f1W,
		parent:       f,
		activeClient: -1,
		g:            geom1,
		leaf:         true,
	}

	f2Frame := &Frame{
		workspace:    f.workspace,
		window:       f2W,
		parent:       f,
		activeClient: -1,
		g:            geom2,
		leaf:         true,
	}

	f.activeClient = -1
	f.leaf = false

	xproto.CreateWindow(
		f.Conn(), ws.Screen.ScreenInfo.RootDepth, f1W, f.window,
		geom1.X, geom1.Y,
		geom1.W, geom1.H,
		0,
		xproto.WindowClassInputOutput, ws.Screen.ScreenInfo.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask,
		[]uint32{
			ws.Screen.ScreenInfo.BlackPixel,
			ws.Color,
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)

	xproto.CreateWindow(
		f.Conn(), ws.Screen.ScreenInfo.RootDepth, f2W, f.window,
		geom2.X, geom2.Y,
		geom2.W, geom2.H,
		0,
		xproto.WindowClassInputOutput, ws.Screen.ScreenInfo.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask,
		[]uint32{
			ws.Screen.ScreenInfo.BlackPixel,
			ws.Color,
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)

	for _, child := range f.children {
		child.parent = f1Frame
	}
	for _, client := range f.clients {
		f1Frame.clients = append(f1Frame.clients, client)
		xproto.ReparentWindow(f.Conn(), client, f1Frame.window, 0, 20)
		mask := uint16(xproto.ConfigWindowX |
			xproto.ConfigWindowY |
			xproto.ConfigWindowWidth |
			xproto.ConfigWindowHeight)
		vals := []uint32{0, 22, uint32(geom1.W), uint32(geom1.H) - 22}
		xproto.ConfigureWindow(f.Conn(), client, mask, vals)
	}
	f.clients = nil
	f.activeClient = -1

	f1Frame.setuptitleBar()
	f2Frame.setuptitleBar()
	f1Frame.Map()
	f2Frame.Map()
	f.children = []*Frame{f1Frame, f2Frame}
	f.workspace.ActiveFrame = f2Frame
	return nil
}

func (f *Frame) splitH() error {
	halfH := f.g.H / 2
	topGeom := Rect{
		X: 0,
		Y: 0,
		W: f.g.W,
		H: halfH,
	}
	bottomGeom := Rect{
		X: 0,
		Y: int16(halfH),
		W: f.g.W,
		H: halfH,
	}
	return f.split(topGeom, bottomGeom)
}

func (f *Frame) splitV() error {
	halfW := f.g.W / 2
	leftGeom := Rect{
		X: 0,
		Y: 0,
		W: halfW,
		H: f.g.H,
	}
	rightGeom := Rect{
		X: int16(halfW),
		Y: 0,
		W: halfW,
		H: f.g.H,
	}
	return f.split(leftGeom, rightGeom)
}

func (f *Frame) AddTab(win xproto.Window) {
	f.clients = append(f.clients, win)
	f.activeClient = len(f.clients) - 1
	f.updateTitleBar()
}

func (f *Frame) updateTitleBar() {
	names := []string{}
	for _, window := range f.clients {
		names = append(names, getWindowName(f.Conn(), window))
	}

	title := fmt.Sprintf(" [%d/%d] %s ", f.activeClient+1, len(f.clients), strings.Join(names, " | "))
	if len(f.clients) == 0 {
		title = " [0/0] empty"
	}
	xproto.ClearArea(f.Conn(), false, f.titleBar, 0, 0, 0, 0)
	xproto.ImageText8(f.Conn(), byte(len(title)), xproto.Drawable(f.titleBar), f.barGC, 0, 14, title)

	if f.workspace.ActiveFrame == f {
		xproto.ChangeWindowAttributes(f.Conn(), f.titleBar, xproto.CwBorderPixel, []uint32{0x335599})
	} else {
		xproto.ChangeWindowAttributes(f.Conn(), f.titleBar, xproto.CwBorderPixel, []uint32{f.workspace.Color})
	}

}

func walk(n *Frame, f func(*Frame)) {
	if n == nil {
		return
	}
	f(n)
	for _, c := range n.children {
		walk(c, f)
	}
}

func (f *Frame) cycleClientLeft() {
	if len(f.clients) <= 1 {
		return
	}
	xproto.UnmapWindow(f.Conn(), f.clients[f.activeClient])
	f.activeClient = (f.activeClient + len(f.clients) - 1) % len(f.clients)
	xproto.MapWindow(f.Conn(), f.clients[f.activeClient])
}

func (f *Frame) cycleClientRight() {
	if len(f.clients) <= 1 {
		return
	}
	xproto.UnmapWindow(f.Conn(), f.clients[f.activeClient])
	f.activeClient = (f.activeClient + 1) % len(f.clients)
	xproto.MapWindow(f.Conn(), f.clients[f.activeClient])
}

func (f *Frame) GetActiveClient() xproto.Window {
	if f.activeClient < 0 || f.activeClient >= len(f.clients) {
		return 0
	}
	return f.clients[f.activeClient]
}

func (f *Frame) RemoveChild(child *Frame) {
	if f == nil || f.Leaf() || len(f.children) != 2 {
		return
	}

	var sibling *Frame
	if f.children[0] == child {
		sibling = f.children[1]
	} else if f.children[1] == child {
		sibling = f.children[0]
	} else {
		return // `child` not found under `f`
	}

	sibling.g = f.g
	sibling.parent = f.parent
	if sibling.parent == nil {
		f.workspace.Root = sibling
	}
	f.workspace.ActiveFrame = sibling
	f.children = nil
	f.clients = nil

	xproto.ReparentWindow(f.Conn(), sibling.GetWindow(), f.workspace.WorkspaceWindow, 0, 0)
	mask := uint16(xproto.ConfigWindowX |
		xproto.ConfigWindowY |
		xproto.ConfigWindowWidth |
		xproto.ConfigWindowHeight)
	vals := []uint32{0, 0, uint32(f.g.W), uint32(f.g.H)}
	xproto.ConfigureWindow(f.Conn(), sibling.GetWindow(), mask, vals)

	f.workspace.updateTitleBars()
	f.Unmap()
	child.Unmap()
}

func (f *Frame) RemoveClient(client xproto.Window) {
	idx := -1
	for i, c := range f.clients {
		if c == client {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}

	f.clients = append(f.clients[:idx], f.clients[idx+1:]...)
	if f.activeClient >= len(f.clients) {
		f.activeClient = len(f.clients) - 1
	}
}
