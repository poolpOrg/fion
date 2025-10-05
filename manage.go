package main

import (
	"fmt"

	"github.com/BurntSushi/xgb/xproto"
)

func (wm *WM) manageExistingWindows() {
	tree, _ := xproto.QueryTree(wm.X, wm.Root).Reply()
	for _, win := range tree.Children {
		//wm.tryManage(win)
		fmt.Println("Existing window:", win)
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
	if attr.MapState == xproto.MapStateUnmapped {
		return
	}
	fmt.Printf("Managing existing window %d\n", w)
	wm.manageWindow(w)
}

func (wm *WM) manageWindow(win xproto.Window) {
	if _, exists := wm.Clients[win]; exists {
		return
	}

	// Make a tab for client
	bw := uint32(1)
	tab, _ := xproto.NewWindowId(wm.X)

	activeWorkspace := wm.ActiveWorkspace()
	parentId := activeWorkspace.GetActiveFrame().Window

	geom, err := xproto.GetGeometry(wm.X, xproto.Drawable(parentId)).Reply()
	if err != nil {
		return
	}
	fmt.Println("Parent geom:", geom, geom.Width, geom.Height, geom.X, geom.Y)

	xproto.CreateWindow(
		wm.X, wm.Scr.RootDepth, tab, parentId,
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
	xproto.ConfigureWindow(wm.X, tab, xproto.ConfigWindowBorderWidth, []uint32{bw})
	xproto.ChangeSaveSet(wm.X, xproto.SetModeInsert, win)
	xproto.ReparentWindow(wm.X, win, tab, 0, 0)
	xproto.MapWindow(wm.X, tab)
	xproto.MapWindow(wm.X, win)

	cl := &Client{Win: win, Tab: tab}
	wm.Clients[win] = cl

	activeWorkspace.GetActiveFrame().AddTab(win)

	// Attach to active Leaf as a new tab
	wm.updateClientList()
	// wm.layoutWorkspace(ws)
}

/*
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
*/
func (wm *WM) updateClientList() {
	buf := make([]byte, 0, 4*len(wm.Clients))
	for w := range wm.Clients {
		buf = append(buf, byte(w), byte(w>>8), byte(w>>16), byte(w>>24))
	}
	xproto.ChangeProperty(wm.X, xproto.PropModeReplace, wm.Root, wm.Atoms.NET_CLIENT_LIST, xproto.AtomWindow, 32, uint32(len(buf)/4), buf)
}
