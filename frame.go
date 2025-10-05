package main

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

type Frame struct {
	Workspace *Workspace

	Window xproto.Window

	Parent   *Frame
	Children []*Frame        // for SplitH/SplitV
	Tabs     []xproto.Window // for Leaf: client windows (tab order)
	Active   int             // index into Tabs
	G        Rect            // assigned geometry during layout
	TitleBar xproto.Window
	BarGC    xproto.Gcontext

	Leaf bool
}

func newRootFrame(ws *Workspace) (*Frame, error) {

	wm := ws.Screen.wm
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
		Workspace: ws,
		Window:    w,
		Parent:    nil,
		Active:    -1,
		G:         frameGeom,
		Leaf:      true,
	}

	xproto.CreateWindow(
		f.Conn(), wm.Scr.RootDepth, w, ws.WorkspaceWindow,
		frameGeom.X, frameGeom.Y, // Position at the bottom
		frameGeom.W, frameGeom.H, // Adjust height for top and bottom borders
		0, // Set border width to 1px
		xproto.WindowClassInputOutput, wm.Scr.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			ws.Screen.ScreenInfo.BlackPixel,
			ws.Color, // Set the border color
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)

	f.setupTitleBar()

	return f, nil
}

func (f *Frame) Conn() *xgb.Conn {
	return f.Workspace.Manager.X
}

func (f *Frame) setupTitleBar() error {
	wm := f.Workspace.Screen.wm
	geom := f.G

	w, err := xproto.NewWindowId(f.Conn())
	if err != nil {
		return err
	}

	xproto.CreateWindow(
		f.Conn(), wm.Scr.RootDepth, w, f.Window,
		0, 0, // Position at the bottom
		geom.W-2, 20, // Adjust height for top and bottom borders
		1, // Set border width to 1px
		xproto.WindowClassInputOutput, wm.Scr.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			f.Workspace.Screen.ScreenInfo.BlackPixel,
			f.Workspace.Color, // Set the border color
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)
	f.TitleBar = w

	// Create a GC and set a core font
	gc, _ := xproto.NewGcontextId(f.Conn())
	xproto.CreateGC(f.Conn(), gc, xproto.Drawable(f.TitleBar),
		xproto.GcForeground|xproto.GcBackground, []uint32{
			f.Workspace.Screen.ScreenInfo.WhitePixel, // text color
			f.Workspace.Screen.ScreenInfo.BlackPixel, // bg (unused by ImageText8)
		},
	)
	// Load a core font and bind it to the GC
	fid, _ := xproto.NewFontId(f.Conn())
	_ = xproto.OpenFontChecked(f.Conn(), fid, uint16(len("fixed")), "fixed").Check()
	xproto.ChangeGC(f.Conn(), gc, xproto.GcFont, []uint32{uint32(fid)})
	f.BarGC = gc

	f.updateTitleBar()
	xproto.MapWindow(f.Conn(), f.TitleBar)

	return nil
}

func (f *Frame) Map() {
	for _, child := range f.Children {
		child.Map()
	}
	xproto.MapWindow(f.Conn(), f.TitleBar)
	xproto.MapWindow(f.Conn(), f.Window)
}

func (f *Frame) Unmap() {
	xproto.UnmapWindow(f.Conn(), f.TitleBar)
	xproto.UnmapWindow(f.Conn(), f.Window)
	for _, child := range f.Children {
		child.Unmap()
	}
}

func (f *Frame) Destroy() {
	xproto.DestroyWindow(f.Conn(), f.Window)
}

func (f *Frame) split(geom1, geom2 Rect) error {
	ws := f.Workspace

	f1W, err := xproto.NewWindowId(f.Conn())
	if err != nil {
		return err
	}

	f2W, err := xproto.NewWindowId(f.Conn())
	if err != nil {
		return err
	}

	f1Frame := &Frame{
		Workspace: f.Workspace,
		Window:    f1W,
		Parent:    f,
		Active:    -1,
		G:         geom1,
		Leaf:      true,
	}

	f2Frame := &Frame{
		Workspace: f.Workspace,
		Window:    f2W,
		Parent:    f,
		Active:    -1,
		G:         geom2,
		Leaf:      true,
	}

	f.Leaf = false

	xproto.CreateWindow(
		f.Conn(), ws.Manager.Scr.RootDepth, f1W, f.Window,
		geom1.X, geom1.Y,
		geom1.W, geom1.H,
		0,
		xproto.WindowClassInputOutput, ws.Manager.Scr.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask,
		[]uint32{
			ws.Screen.ScreenInfo.BlackPixel,
			ws.Color,
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)

	xproto.CreateWindow(
		f.Conn(), ws.Manager.Scr.RootDepth, f2W, f.Window,
		geom2.X, geom2.Y,
		geom2.W, geom2.H,
		0,
		xproto.WindowClassInputOutput, ws.Manager.Scr.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask,
		[]uint32{
			ws.Screen.ScreenInfo.BlackPixel,
			ws.Color,
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)

	for _, child := range f.Children {
		child.Parent = f1Frame
	}
	for _, client := range f.Tabs {
		f1Frame.Tabs = append(f1Frame.Tabs, client)
		xproto.ReparentWindow(f.Conn(), client, f1Frame.Window, 0, 20)
		mask := uint16(xproto.ConfigWindowX |
			xproto.ConfigWindowY |
			xproto.ConfigWindowWidth |
			xproto.ConfigWindowHeight)
		vals := []uint32{0, 22, uint32(geom1.W), uint32(geom1.H) - 22}
		xproto.ConfigureWindow(f.Conn(), client, mask, vals)
	}
	f.Tabs = nil
	f.Active = -1

	f1Frame.setupTitleBar()
	f2Frame.setupTitleBar()
	f1Frame.Map()
	f2Frame.Map()
	f.Children = []*Frame{f1Frame, f2Frame}
	f.Workspace.ActiveFrame = f2Frame
	return nil
}

func (f *Frame) splitH() error {
	halfH := f.G.H / 2
	topGeom := Rect{
		X: 0,
		Y: 0,
		W: f.G.W,
		H: halfH,
	}
	bottomGeom := Rect{
		X: 0,
		Y: int16(halfH),
		W: f.G.W,
		H: halfH,
	}
	return f.split(topGeom, bottomGeom)
}

func (f *Frame) splitV() error {
	halfW := f.G.W / 2
	leftGeom := Rect{
		X: 0,
		Y: 0,
		W: halfW,
		H: f.G.H,
	}
	rightGeom := Rect{
		X: int16(halfW),
		Y: 0,
		W: halfW,
		H: f.G.H,
	}
	return f.split(leftGeom, rightGeom)
}

func (f *Frame) AddTab(win xproto.Window) {
	f.Tabs = append(f.Tabs, win)
	f.Active = len(f.Tabs) - 1
	f.updateTitleBar()
}

func (f *Frame) updateTitleBar() {
	names := []string{}
	for _, window := range f.Tabs {
		names = append(names, getWindowName(f.Conn(), window))
	}

	title := fmt.Sprintf(" [%d/%d] %s ", f.Active+1, len(f.Tabs), strings.Join(names, " | "))
	if len(f.Tabs) == 0 {
		title = " [0/0] empty"
	}
	xproto.ClearArea(f.Conn(), false, f.TitleBar, 0, 0, 0, 0)
	xproto.ImageText8(f.Conn(), byte(len(title)), xproto.Drawable(f.TitleBar), f.BarGC, 0, 14, title)

	if f.Workspace.ActiveFrame == f {
		xproto.ChangeWindowAttributes(f.Conn(), f.TitleBar, xproto.CwBorderPixel, []uint32{0xff0000})
	} else {
		xproto.ChangeWindowAttributes(f.Conn(), f.TitleBar, xproto.CwBorderPixel, []uint32{f.Workspace.Color})
	}

}

func walk(n *Frame, f func(*Frame)) {
	if n == nil {
		return
	}
	f(n)
	for _, c := range n.Children {
		walk(c, f)
	}
}

func (f *Frame) cycleClientLeft() {
	if len(f.Tabs) <= 1 {
		return
	}
	xproto.UnmapWindow(f.Conn(), f.Tabs[f.Active])
	f.Active = (f.Active + len(f.Tabs) - 1) % len(f.Tabs)
	xproto.MapWindow(f.Conn(), f.Tabs[f.Active])
}

func (f *Frame) cycleClientRight() {
	if len(f.Tabs) <= 1 {
		return
	}
	xproto.UnmapWindow(f.Conn(), f.Tabs[f.Active])
	f.Active = (f.Active + 1) % len(f.Tabs)
	xproto.MapWindow(f.Conn(), f.Tabs[f.Active])
}
