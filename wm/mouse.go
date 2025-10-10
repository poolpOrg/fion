package wm

/*
func setDefaultCursor(X *xgb.Conn, win xproto.Window) {
	const cursorLeftPtr = 68
	fid, _ := xproto.NewFontId(X)
	_ = xproto.OpenFontChecked(X, fid, uint16(len("cursor")), "cursor").Check()
	cid, _ := xproto.NewCursorId(X)
	_ = xproto.CreateGlyphCursorChecked(X, cid, fid, fid, uint16(cursorLeftPtr), uint16(cursorLeftPtr+1), 0xffff, 0xffff, 0xffff, 0, 0, 0).Check()
	_ = xproto.ChangeWindowAttributesChecked(X, win, xproto.CwCursor, []uint32{uint32(cid)}).Check()
	_ = xproto.CloseFontChecked(X, fid).Check()
}

func (wm *WM) beginDragMove(cl *Client, start xproto.ButtonPressEvent) {
	rootStartX, rootStartY := int16(start.RootX), int16(start.RootY)
	geom, _ := xproto.GetGeometry(wm.X, xproto.Drawable(cl.Win)).Reply()
	fx0, fy0 := int16(geom.X), int16(geom.Y)
	wm.grabPointer(wm.Root)
	for {
		ev, err := wm.X.WaitForEvent()
		if err != nil {
			break
		}
		switch e := ev.(type) {
		case xproto.MotionNotifyEvent:
			dx := int16(e.RootX) - rootStartX
			dy := int16(e.RootY) - rootStartY
			xproto.ConfigureWindow(wm.X, cl.Win, xproto.ConfigWindowX|xproto.ConfigWindowY,
				[]uint32{uint32(int16(fx0 + dx)), uint32(int16(fy0 + dy))})
		case xproto.ButtonReleaseEvent:
			wm.ungrabPointer()
			return
		}
	}
	wm.ungrabPointer()
}

func (wm *WM) beginDragResize(cl *Client, start xproto.ButtonPressEvent) {
	rootStartX, rootStartY := int16(start.RootX), int16(start.RootY)
	geom, _ := xproto.GetGeometry(wm.X, xproto.Drawable(cl.Win)).Reply()
	fw0, fh0 := int16(geom.Width), int16(geom.Height)
	wm.grabPointer(wm.Root)
	for {
		ev, err := wm.X.WaitForEvent()
		if err != nil {
			break
		}
		switch e := ev.(type) {
		case xproto.MotionNotifyEvent:
			dx := int16(e.RootX) - rootStartX
			dy := int16(e.RootY) - rootStartY
			w := uint32(math.Max(50, float64(fw0+dx)))
			h := uint32(math.Max(50, float64(fh0+dy)))
			xproto.ConfigureWindow(wm.X, cl.Win, xproto.ConfigWindowWidth|xproto.ConfigWindowHeight, []uint32{w, h})
		case xproto.ButtonReleaseEvent:
			wm.ungrabPointer()
			return
		}
	}
	wm.ungrabPointer()
}

func (wm *WM) grabPointer(root xproto.Window) {
	xproto.GrabPointer(wm.X, false, root, xproto.EventMaskButtonRelease|xproto.EventMaskPointerMotion, xproto.GrabModeAsync, xproto.GrabModeAsync, root, 0, xproto.TimeCurrentTime)
}
func (wm *WM) ungrabPointer() { xproto.UngrabPointer(wm.X, xproto.TimeCurrentTime) }
*/
