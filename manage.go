package main

import (
	"fmt"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

func NewWM() (*WM, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}

	setup := xproto.Setup(conn)
	if setup == nil || len(setup.Roots) == 0 {
		conn.Close()
		return nil, fmt.Errorf("no X screens found")
	}

	scr := setup.DefaultScreen(conn)
	wm := &WM{
		X:     conn,
		Setup: setup,

		Scr:     scr,
		Root:    scr.Root,
		Atoms:   getAtoms(conn),
		NumLock: detectNumLockMask(conn),
		Frames:  make(map[xproto.Window]*FrameNode),
		Clients: make(map[xproto.Window]*Client),
	}

	mask := uint32(
		xproto.EventMaskSubstructureRedirect |
			xproto.EventMaskSubstructureNotify |
			xproto.EventMaskPropertyChange |
			xproto.EventMaskButtonPress |
			xproto.EventMaskButtonRelease |
			xproto.EventMaskPointerMotion |
			xproto.EventMaskKeyPress,
	)
	if err := xproto.ChangeWindowAttributesChecked(conn, wm.Root, xproto.CwEventMask, []uint32{mask}).Check(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("another WM running: %w", err)
	}
	xproto.ChangeWindowAttributes(wm.X, wm.Root, xproto.CwBackPixel, []uint32{wm.ColNormal})
	xproto.ClearArea(wm.X, false, wm.Root, 0, 0, wm.Scr.WidthInPixels, wm.Scr.HeightInPixels)

	setDefaultCursor(conn, wm.Root)

	if err := wm.initEWMH(); err != nil {
		return nil, err
	}

	wm.initScreens()

	return wm, nil
}

func (wm *WM) manageExistingWindows() {
	tree, _ := xproto.QueryTree(wm.X, wm.Root).Reply()
	for _, win := range tree.Children {
		wm.tryManage(win)
		//fmt.Println("Existing window:", win)
	}
}

func (wm *WM) Close() {
	if wm.X != nil {
		wm.X.Close()
	}
}

func (wm *WM) tryManage(w xproto.Window) {
	attr, err := xproto.GetWindowAttributes(wm.X, w).Reply()
	if err != nil {
		return
	}
	if attr.OverrideRedirect {
		return
	}
	//	if attr.MapState == xproto.MapStateUnmapped {
	//		return
	//	}
	fmt.Printf("Managing existing window %d\n", w)
	wm.manageWindow(w)
}

func (wm *WM) manageWindow(win xproto.Window) {
	if _, exists := wm.Clients[win]; exists {
		return
	}

	// Make a frame for client
	bw := uint32(1)
	frame, _ := xproto.NewWindowId(wm.X)

	activeWorkspace := wm.ActiveWorkspace()
	parentId := activeWorkspace.Area

	geom, err := xproto.GetGeometry(wm.X, xproto.Drawable(parentId)).Reply()
	if err != nil {
		return
	}
	fmt.Println("Parent geom:", geom, geom.Width, geom.Height, geom.X, geom.Y)

	xproto.CreateWindow(
		wm.X, wm.Scr.RootDepth, frame, parentId,
		0, 0,
		geom.Width, geom.Height, 0,
		xproto.WindowClassInputOutput, wm.Scr.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask,
		[]uint32{
			wm.Scr.BlackPixel,
			wm.ColNormal, // <-- random border color
			xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskPointerMotion,
		},
	)
	xproto.ConfigureWindow(wm.X, frame, xproto.ConfigWindowBorderWidth, []uint32{bw})
	xproto.ChangeSaveSet(wm.X, xproto.SetModeInsert, win)
	xproto.ReparentWindow(wm.X, win, frame, 0, 0)
	xproto.MapWindow(wm.X, frame)
	xproto.MapWindow(wm.X, win)

	cl := &Client{Win: win, Frame: frame}
	wm.Clients[win] = cl

	// Attach to active Leaf as a new tab
	ws := wm.ActiveWorkspace()
	leaf := ws.FocusedFrame
	if leaf == nil {
		leaf = ws.Root
	}
	if leaf.Kind != Leaf { // create a leaf if focus isn't a leaf
		leaf = &FrameNode{Kind: Leaf, Parent: leaf, Tabs: nil, Active: -1, G: leaf.G}
		leaf.Parent.Children = append(leaf.Parent.Children, leaf)
	}
	leaf.Tabs = append(leaf.Tabs, win)
	leaf.Active = len(leaf.Tabs) - 1
	wm.Frames[frame] = leaf

	wm.updateClientList()
	wm.layoutWorkspace(ws)
}

func (wm *WM) unmanageWindow(win xproto.Window) {
	cl, ok := wm.Clients[win]
	if !ok {
		return
	}
	ws := wm.ActiveWorkspace()
	// Remove from leaf tabs
	if leaf, ok := wm.Frames[cl.Frame]; ok {
		idx := -1
		for i, w := range leaf.Tabs {
			if w == win {
				idx = i
				break
			}
		}
		if idx >= 0 {
			leaf.Tabs = append(leaf.Tabs[:idx], leaf.Tabs[idx+1:]...)
			if leaf.Active >= len(leaf.Tabs) {
				leaf.Active = len(leaf.Tabs) - 1
			}
		}
		delete(wm.Frames, cl.Frame)
	}
	// Unparent and destroy frame
	xproto.UnmapWindow(wm.X, cl.Frame)
	xproto.ReparentWindow(wm.X, cl.Win, wm.Root, 0, 0)
	xproto.DestroyWindow(wm.X, cl.Frame)
	delete(wm.Clients, win)

	wm.updateClientList()
	wm.layoutWorkspace(ws)
}

func (wm *WM) updateClientList() {
	buf := make([]byte, 0, 4*len(wm.Clients))
	for w := range wm.Clients {
		buf = append(buf, byte(w), byte(w>>8), byte(w>>16), byte(w>>24))
	}
	xproto.ChangeProperty(wm.X, xproto.PropModeReplace, wm.Root, wm.Atoms.NET_CLIENT_LIST, xproto.AtomWindow, 32, uint32(len(buf)/4), buf)
}

func (wm *WM) layoutWorkspace(ws *Workspace) {
	// total screen geom
	g := ws.Screen.Geom()
	// leave space for the bar
	below := Rect{X: g.X, Y: g.Y + int16(BarHeight), W: g.W, H: g.H - BarHeight}

	// position bar (in case of resize)
	xproto.ConfigureWindow(
		wm.X, ws.Bar,
		xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
		[]uint32{uint32(g.X), uint32(g.Y), uint32(g.W), uint32(BarHeight)},
	)
	xproto.MapWindow(wm.X, ws.Bar)

	// assign and apply layout for tiling area
	assignRect(ws.Root, below)
	wm.applyLayout(ws.Root)
}
