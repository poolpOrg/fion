package wm

import (
	"fmt"

	"github.com/BurntSushi/xgb/xproto"
)

type Rect struct {
	X, Y int16
	W, H uint16
}

//func (wm *WM) manageExistingWindows() {
//	tree, _ := xproto.QueryTree(wm.X, wm.Root).Reply()
//	for _, win := range tree.Children {
//		//wm.tryManage(win)
//		_ = win
//	}
//}

func (wm *WM) tryManage(w xproto.Window) {
	attr, err := xproto.GetWindowAttributes(wm.Conn(), w).Reply()
	if err != nil {
		return
	}
	if attr.OverrideRedirect {
		return
	}
	if attr.MapState != xproto.MapStateUnmapped {
		return
	}
	wm.manageWindow(w)
}

func (wm *WM) manageWindow(win xproto.Window) {
	if _, exists := wm.Clients[win]; exists {
		return
	}

	// Make a tab for client
	bw := uint32(1)

	activeWorkspace := wm.GetActiveWorkspace()
	parentId := activeWorkspace.GetActiveFrame().GetWindow()

	geom, err := xproto.GetGeometry(wm.Conn(), xproto.Drawable(parentId)).Reply()
	if err != nil {
		return
	}
	fmt.Println("Parent geom:", geom, geom.Width, geom.Height, geom.X, geom.Y)

	xproto.ConfigureWindow(wm.Conn(), win, xproto.ConfigWindowBorderWidth, []uint32{bw})
	xproto.ChangeSaveSet(wm.Conn(), xproto.SetModeInsert, win)
	xproto.ReparentWindow(wm.Conn(), win, parentId, 0, 20)

	mask := uint16(xproto.ConfigWindowX |
		xproto.ConfigWindowY |
		xproto.ConfigWindowWidth |
		xproto.ConfigWindowHeight)
	vals := []uint32{0, 22, uint32(geom.Width), uint32(geom.Height) - 22}
	xproto.ConfigureWindow(wm.Conn(), win, mask, vals)

	//xproto.ChangeWindowAttributes(wm.Conn(), win, xproto.CwBorderPixel, []uint32{activeWorkspace.Color})
	//xproto.ConfigureWindow(wm.Conn(), win, xproto.ConfigWindowBorderWidth, []uint32{1})

	xproto.MapWindow(wm.Conn(), win)

	wm.Clients[win] = struct{}{}

	activeWorkspace.GetActiveFrame().AddTab(win)

	// Attach to active Leaf as a new tab
	//wm.updateClientList()
	// wm.layoutWorkspace(ws)
}

func (wm *WM) unmanageWindow(win xproto.Window) {
	if _, ok := wm.Clients[win]; !ok {
		return
	}
	xproto.UnmapWindow(wm.Conn(), win)
	xproto.DestroyWindow(wm.Conn(), win)
	delete(wm.Clients, win)
}

//func (wm *WM) updateClientList() {
//	buf := make([]byte, 0, 4*len(wm.Clients))
//	for w := range wm.Clients {
//		buf = append(buf, byte(w), byte(w>>8), byte(w>>16), byte(w>>24))
//	}
//	xproto.ChangeProperty(wm.X, xproto.PropModeReplace, wm.Root, wm.Atoms.NET_CLIENT_LIST, xproto.AtomWindow, 32, uint32(len(buf)/4), buf)
//}
