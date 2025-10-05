package main

type Rect struct {
	X, Y int16
	W, H uint16
}

/*
func (wm *WM) applyLayout(n *FrameNode) {

	if n.Kind == Leaf {
		bw := uint32(3)

		if len(n.Tabs) == 0 {
			// Ensure a visible placeholder frame
			if n.Ghost == 0 {
				ghost, _ := xproto.NewWindowId(wm.X)
				xproto.CreateWindow(
					wm.X, wm.Scr.RootDepth, ghost, wm.Root,
					n.G.X, n.G.Y, n.G.W, n.G.H, 0,
					xproto.WindowClassInputOutput, wm.Scr.RootVisual,
					xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask,
					[]uint32{
						wm.Scr.BlackPixel, // background
						wm.ColNormal,      // border color
						xproto.EventMaskButtonPress,
					},
				)
				// set border width
				xproto.ConfigureWindow(wm.X, ghost, xproto.ConfigWindowBorderWidth, []uint32{bw})
				n.Ghost = ghost
			}
			// position + show ghost
			xproto.ConfigureWindow(
				wm.X, n.Ghost,
				xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
				[]uint32{uint32(n.G.X), uint32(n.G.Y), uint32(n.G.W), uint32(n.G.H)},
			)
			xproto.MapWindow(wm.X, n.Ghost)
			return
		}

		// There ARE tabs: hide ghost (if any) and show only active tab's frame
		if n.Ghost != 0 {
			xproto.UnmapWindow(wm.X, n.Ghost)
		}
		for i, w := range n.Tabs {
			frame, ok := wm.frameFor(w)
			if !ok {
				continue
			}
			if i == n.Active {
				xproto.ConfigureWindow(wm.X, frame,
					xproto.ConfigWindowX|xproto.ConfigWindowY|xproto.ConfigWindowWidth|xproto.ConfigWindowHeight,
					[]uint32{uint32(n.G.X), uint32(n.G.Y), uint32(n.G.W), uint32(n.G.H)},
				)
				xproto.MapWindow(wm.X, frame)
				xproto.MapWindow(wm.X, w)
			} else {
				xproto.UnmapWindow(wm.X, w)
			}
		}
		return
	}
}
*/

// frameFor returns (frameWindow, ok)
/*
func (wm *WM) frameFor(win xproto.Window) (xproto.Window, bool) {
	if cl, ok := wm.Clients[win]; ok {
		return cl.Frame, true
	}
	return 0, false
}
*/

// -------------------------- Commands (Ion-style) --------------------------

/*
func (wm *WM) FocusNextTab() {
	ws := wm.ActiveWorkspace()
	f := ws.FocusedFrame
	if f == nil || f.Kind != Leaf || len(f.Tabs) == 0 {
		return
	}
	f.Active = (f.Active + 1) % len(f.Tabs)
	wm.layoutWorkspace(ws)
}
*/

/*
func (wm *WM) handleConfigure(e xproto.ConfigureRequestEvent) {
	// If framed in a leaf, move/resize the frame; otherwise pass-through
	if leaf := wm.Frames[e.Window]; leaf != nil {
		bw := int16(2)
		gx, gy := int16(e.X), int16(e.Y)
		gw, gh := e.Width+uint16(2*bw), e.Height+uint16(2*bw)
		mask := uint16(0)
		vals := make([]uint32, 0, 4)
		if e.ValueMask&xproto.ConfigWindowX != 0 {
			mask |= xproto.ConfigWindowX
			vals = append(vals, uint32(gx-bw))
		}
		if e.ValueMask&xproto.ConfigWindowY != 0 {
			mask |= xproto.ConfigWindowY
			vals = append(vals, uint32(gy-bw))
		}
		if e.ValueMask&xproto.ConfigWindowWidth != 0 {
			mask |= xproto.ConfigWindowWidth
			vals = append(vals, uint32(gw))
		}
		if e.ValueMask&xproto.ConfigWindowHeight != 0 {
			mask |= xproto.ConfigWindowHeight
			vals = append(vals, uint32(gh))
		}
		xproto.ConfigureWindow(wm.X, wm.clientByLeaf(leaf).Frame, mask, vals)
		return
	}
	// unmanaged
	values := []uint32{}
	mask := uint16(0)
	if e.ValueMask&xproto.ConfigWindowX != 0 {
		mask |= xproto.ConfigWindowX
		values = append(values, uint32(e.X))
	}
	if e.ValueMask&xproto.ConfigWindowY != 0 {
		mask |= xproto.ConfigWindowY
		values = append(values, uint32(e.Y))
	}
	if e.ValueMask&xproto.ConfigWindowWidth != 0 {
		mask |= xproto.ConfigWindowWidth
		values = append(values, uint32(e.Width))
	}
	if e.ValueMask&xproto.ConfigWindowHeight != 0 {
		mask |= xproto.ConfigWindowHeight
		values = append(values, uint32(e.Height))
	}
	if e.ValueMask&xproto.ConfigWindowBorderWidth != 0 {
		mask |= xproto.ConfigWindowBorderWidth
		values = append(values, uint32(e.BorderWidth))
	}
	if e.ValueMask&xproto.ConfigWindowSibling != 0 {
		mask |= xproto.ConfigWindowSibling
		values = append(values, uint32(e.Sibling))
	}
	if e.ValueMask&xproto.ConfigWindowStackMode != 0 {
		mask |= xproto.ConfigWindowStackMode
		values = append(values, uint32(e.StackMode))
	}
	xproto.ConfigureWindow(wm.X, e.Window, mask, values)
}
*/

/*
	func (wm *WM) handleClientMessage(e xproto.ClientMessageEvent) {
		if e.Type == wm.Atoms.NET_ACTIVE_WINDOW {
			w := xproto.Window(e.Data.Data32[0])
			// Focus: make it the active tab in its leaf
			for _, s := range wm.Screens {
				for _, ws := range s.Workspaces {
					walk(ws.Root, func(n *FrameNode) {
						if n.Kind == Leaf {
							for i, tw := range n.Tabs {
								if tw == w {
									n.Active = i
								}
							}
						}
					})
				}
			}
			wm.layoutWorkspace(wm.ActiveWorkspace())
		}
	}
*/

/*
func walk(n *FrameNode, f func(*FrameNode)) {
	if n == nil {
		return
	}
	f(n)
	for _, c := range n.Children {
		walk(c, f)
	}
}
*/
/*
func (wm *WM) setFrameBorder(cl *Client, pixel uint32) {
	if cl == nil {
		return
	}
	xproto.ChangeWindowAttributes(wm.X, cl.Frame, xproto.CwBorderPixel, []uint32{pixel})
}

func (wm *WM) focusClient(cl *Client) {
	if cl == nil {
		return
	}
	// unfocus old
	if wm.Focused != nil && wm.Focused != cl {
		wm.setFrameBorder(wm.Focused, wm.ColNormal)
	}
	// focus new
	wm.setFrameBorder(cl, wm.ColFocused)
	xproto.SetInputFocus(wm.X, xproto.InputFocusPointerRoot, cl.Win, xproto.TimeCurrentTime)
	wm.Focused = cl
}
*/
/*
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
*/
