package wm

import (
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type Frame struct {
	screen    *Screen
	workspace *Workspace // nil for the scratchpad
	window    xproto.Window

	parent       *Frame
	children     []*Frame        // for SplitH/SplitV
	clients      []xproto.Window // for Leaf: client windows (tab order)
	activeClient int             // index into Tabs

	g        Geometry // relative to the parent window, applied by layout
	titleBar xproto.Window
	barGC    xproto.Gcontext

	leaf bool

	// for a split frame: children side by side (splitV) rather than
	// stacked (splitH)
	vertical bool
}

func newRootFrame(ws *Workspace) (*Frame, error) {
	geom := ws.Screen.Geometry()
	// leave room for the info bar at the bottom
	return newFrame(ws.Screen, ws, nil, Geometry{X: 0, Y: 0, W: geom.W, H: geom.H - uint16(infoBarOuterH())})
}

// newFrame creates an empty leaf frame at g inside parent, at the top of the
// workspace ws when parent is nil, or floating on the screen's root when ws
// is nil too. The frame is left unmapped.
func newFrame(screen *Screen, ws *Workspace, parent *Frame, g Geometry) (*Frame, error) {
	w, err := xproto.NewWindowId(screen.Conn())
	if err != nil {
		return nil, err
	}

	f := &Frame{
		screen:       screen,
		workspace:    ws,
		window:       w,
		parent:       parent,
		activeClient: -1,
		g:            g,
		leaf:         true,
	}

	borderWidth := uint16(0)
	if f.floating() {
		borderWidth = 1
	}
	xproto.CreateWindow(
		f.Conn(), screen.Info().RootDepth, w, f.parentWindow(),
		g.X, g.Y,
		g.W, g.H,
		borderWidth,
		xproto.WindowClassInputOutput, screen.Info().RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask,
		[]uint32{
			colorEmpty,
			colorAccent, // the scratchpad's border
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)

	if err := f.setuptitleBar(); err != nil {
		xproto.DestroyWindow(f.Conn(), w)
		return nil, err
	}
	return f, nil
}

// parentWindow is the X window the frame's window is a child of.
func (f *Frame) parentWindow() xproto.Window {
	switch {
	case f.parent != nil:
		return f.parent.window
	case f.floating():
		return f.screen.Info().Root
	}
	return f.workspace.WorkspaceWindow
}

// floating reports whether f floats above the workspaces rather than being
// tiled in one: the scratchpad.
func (f *Frame) floating() bool {
	return f.workspace == nil
}

// firstLeaf is the top-left leaf of the subtree rooted at f.
func (f *Frame) firstLeaf() *Frame {
	for !f.leaf {
		f = f.children[0]
	}
	return f
}

func (f *Frame) Conn() *xgb.Conn {
	return f.wm().Conn()
}

func (f *Frame) wm() *Manager {
	return f.screen.wm
}

// isActive reports whether f is the frame the bindings act on: the
// scratchpad when it is shown, the workspace's active frame otherwise.
func (f *Frame) isActive() bool {
	if f.floating() {
		return f.screen.scratchpadShown
	}
	return f.workspace.ActiveFrame == f && !f.screen.scratchpadShown
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
	scr := f.screen.Info()
	geom := f.g

	w, err := xproto.NewWindowId(f.Conn())
	if err != nil {
		return err
	}

	xproto.CreateWindow(
		f.Conn(), scr.RootDepth, w, f.window,
		0, 0, // Position at the bottom
		geom.W-2, uint16(titleH()-2), // inside its border
		1, // Set border width to 1px
		xproto.WindowClassInputOutput, scr.RootVisual,
		xproto.CwBackPixel|xproto.CwBorderPixel|xproto.CwEventMask, // Add CwBorderPixel
		[]uint32{
			colorBar,
			colorBorder,
			xproto.EventMaskExposure | xproto.EventMaskButtonPress,
		},
	)
	f.titleBar = w

	// Create a GC and set a core font
	gc, _ := xproto.NewGcontextId(f.Conn())
	xproto.CreateGC(f.Conn(), gc, xproto.Drawable(f.titleBar),
		xproto.GcForeground|xproto.GcBackground, []uint32{
			colorText, // text color
			colorBar,  // background, of the text drawn by ImageText8
		},
	)
	// bind the font; the GC keeps it alive, the id is no longer needed
	if fid, ok := openFont(f.Conn(), font.plain); ok {
		xproto.ChangeGC(f.Conn(), gc, xproto.GcFont, []uint32{uint32(fid)})
		xproto.CloseFont(f.Conn(), fid)
	}
	f.barGC = gc
	f.wm().Frames[f.titleBar] = f

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

// Destroy destroys the frame's window, and with it those of its subtree,
// and frees the subtree's graphics contexts.
func (f *Frame) Destroy() {
	walk(f, func(n *Frame) {
		xproto.FreeGC(n.Conn(), n.barGC)
		delete(n.wm().Frames, n.titleBar)
	})
	xproto.DestroyWindow(f.Conn(), f.window)
}

// layout applies f.g to the frame's window and carries it down to its title
// bar, its clients and its children.
func (f *Frame) layout() {
	mask := uint16(xproto.ConfigWindowX |
		xproto.ConfigWindowY |
		xproto.ConfigWindowWidth |
		xproto.ConfigWindowHeight)

	xproto.ConfigureWindow(f.Conn(), f.window, mask,
		[]uint32{uint32(f.g.X), uint32(f.g.Y), uint32(f.g.W), uint32(f.g.H)})
	xproto.ConfigureWindow(f.Conn(), f.titleBar, xproto.ConfigWindowWidth,
		[]uint32{uint32(f.g.W) - 2})

	for _, client := range f.clients {
		xproto.ConfigureWindow(f.Conn(), client, mask, f.clientGeometry())
	}

	if !f.leaf {
		f.children[0].g, f.children[1].g = f.childGeometries()
		for _, child := range f.children {
			child.layout()
		}
	}
}

// clientGeometry is where a client goes in f, below the title bar, in f's
// coordinates, as ConfigureWindow's x, y, width and height: its 1px
// border included, it fills the frame.
func (f *Frame) clientGeometry() []uint32 {
	return []uint32{0, uint32(titleH()), uint32(max(int(f.g.W)-2, 1)), uint32(max(int(f.g.H)-titleH()-2, 1))}
}

// childGeometries divides a split frame between its two children, in the
// frame's own coordinates.
func (f *Frame) childGeometries() (Geometry, Geometry) {
	if f.vertical {
		w := f.g.W / 2
		return Geometry{X: 0, Y: 0, W: w, H: f.g.H},
			Geometry{X: int16(w), Y: 0, W: f.g.W - w, H: f.g.H}
	}
	h := f.g.H / 2
	return Geometry{X: 0, Y: 0, W: f.g.W, H: h},
		Geometry{X: 0, Y: int16(h), W: f.g.W, H: f.g.H - h}
}

// split turns the leaf f into a split frame holding two new leaves, side
// by side when vertical, stacked otherwise: one takes over f's clients, the
// other, the first when newFirst, is empty and becomes active.
func (f *Frame) split(vertical, newFirst bool) error {
	if f.floating() {
		return fmt.Errorf("the scratchpad can't be split")
	}
	if !f.leaf {
		return fmt.Errorf("frame is already split")
	}
	if (vertical && f.g.W < 256) || (!vertical && f.g.H < 256) {
		return fmt.Errorf("frame too small to split")
	}

	f.vertical = vertical
	g1, g2 := f.childGeometries()
	f1, err := newFrame(f.screen, f.workspace, f, g1)
	if err != nil {
		return err
	}
	f2, err := newFrame(f.screen, f.workspace, f, g2)
	if err != nil {
		f1.Destroy()
		return err
	}

	keep, fresh := f1, f2
	if newFirst {
		keep, fresh = f2, f1
	}
	for _, client := range f.clients {
		if c, ok := f.wm().Clients[client]; ok {
			c.frame = keep
			// reparenting a mapped window unmaps it first
			if c.mapped {
				c.ignoreUnmap++
			}
		}
		xproto.ReparentWindow(f.Conn(), client, keep.window, 0, int16(titleH()))
	}
	keep.clients, keep.activeClient = f.clients, f.activeClient
	f.clients, f.activeClient = nil, -1

	f.leaf = false
	f.children = []*Frame{f1, f2}
	keep.layout()
	f1.Map()
	f2.Map()

	f.workspace.ActiveFrame = fresh
	f.workspace.updateTitleBars()
	return nil
}

func (f *Frame) splitH() error {
	return f.split(false, false)
}

func (f *Frame) splitV() error {
	return f.split(true, false)
}

// AddTab adds a client to the leaf f as its active tab.
func (f *Frame) AddTab(win xproto.Window) {
	f.clients = append(f.clients, win)
	f.selectClient(len(f.clients) - 1)
}

const ()

// tabWidth is the width of each tab in the title bar, but the last, which
// takes what is left.
func (f *Frame) tabWidth() int {
	if len(f.clients) == 0 {
		return 0
	}
	return (int(f.g.W) - 2) / len(f.clients)
}

// tabAt returns the index of the tab at x in the title bar, or -1.
func (f *Frame) tabAt(x int16) int {
	w := f.tabWidth()
	if w == 0 || x < 0 {
		return -1
	}
	if int(x) >= int(f.g.W)-2 {
		return -1
	}
	return min(int(x)/w, len(f.clients)-1)
}

func (f *Frame) updateTitleBar() {
	conn, bar := f.Conn(), xproto.Drawable(f.titleBar)
	xproto.ClearArea(conn, false, f.titleBar, 0, 0, 0, 0)

	text := func(x int16, s string, fg, bg uint32) {
		if len(s) > 255 {
			s = s[:255]
		}
		xproto.ChangeGC(conn, f.barGC, xproto.GcForeground|xproto.GcBackground, []uint32{fg, bg})
		xproto.ImageText8(conn, byte(len(s)), bar, f.barGC, x, int16(baseline(titleH()-2)), s)
	}

	if len(f.clients) == 0 {
		text(4, "empty", colorDim, colorBar)
	}

	w := f.tabWidth()
	for i, client := range f.clients {
		bg, fg := uint32(colorTab), uint32(colorText)
		if i == f.activeClient {
			bg = colorTabSelected
			if f.isActive() {
				bg, fg = colorAccent, colorAccentText
			}
		}
		x := int16(i * w)
		// a 1px gap between tabs, the last one taking what is left
		tw := w - 1
		if i == len(f.clients)-1 {
			tw = int(f.g.W) - 2 - i*w
		}
		xproto.ChangeGC(conn, f.barGC, xproto.GcForeground, []uint32{bg})
		xproto.PolyFillRectangle(conn, bar, f.barGC,
			[]xproto.Rectangle{{X: x, Y: 0, Width: uint16(max(tw, 1)), Height: uint16(titleH() - 2)}})

		title := getWindowName(conn, client)
		if n := (w - 8) / charW(); len(title) > n {
			title = title[:max(n, 0)]
		}
		text(x+4, title, fg, bg)
	}

	if f.isActive() {
		xproto.ChangeWindowAttributes(conn, f.titleBar, xproto.CwBorderPixel, []uint32{colorAccent})
	} else {
		xproto.ChangeWindowAttributes(conn, f.titleBar, xproto.CwBorderPixel, []uint32{colorBorder})
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
	if len(f.clients) > 1 {
		f.selectClient((f.activeClient + len(f.clients) - 1) % len(f.clients))
	}
}

func (f *Frame) cycleClientRight() {
	if len(f.clients) > 1 {
		f.selectClient((f.activeClient + 1) % len(f.clients))
	}
}

// selectClient makes the i-th tab the active one.
func (f *Frame) selectClient(i int) {
	if i < 0 || i >= len(f.clients) {
		return
	}
	f.activeClient = i
	f.showActiveClient()
	f.updateTitleBar()
	if f.floating() {
		f.wm().updateFocus()
	}
}

// showActiveClient maps the active tab and unmaps the others.
func (f *Frame) showActiveClient() {
	// map before unmapping, so that the frame doesn't flash empty
	if active := f.GetActiveClient(); active != 0 {
		if c, ok := f.wm().Clients[active]; ok && !c.mapped {
			xproto.MapWindow(f.Conn(), active)
			c.mapped = true
		}
	}
	for i, client := range f.clients {
		if i == f.activeClient {
			continue
		}
		if c, ok := f.wm().Clients[client]; ok && c.mapped {
			c.ignoreUnmap++
			xproto.UnmapWindow(f.Conn(), client)
			c.mapped = false
		}
	}
}

func (f *Frame) GetActiveClient() xproto.Window {
	if f.activeClient < 0 || f.activeClient >= len(f.clients) {
		return 0
	}
	return f.clients[f.activeClient]
}

// RemoveChild removes the empty leaf child from the split frame f. Its
// sibling takes f's place in the tree and grows into f's geometry.
func (f *Frame) RemoveChild(child *Frame) error {
	if f.leaf || len(f.children) != 2 {
		return fmt.Errorf("frame is not split")
	}

	var sibling *Frame
	switch child {
	case f.children[0]:
		sibling = f.children[1]
	case f.children[1]:
		sibling = f.children[0]
	default:
		return fmt.Errorf("frame is not a child of this frame")
	}
	if !child.leaf || len(child.clients) != 0 {
		return fmt.Errorf("frame is not empty")
	}

	ws := f.workspace
	sibling.parent = f.parent
	sibling.g = f.g
	if f.parent == nil {
		ws.Root = sibling
	} else {
		for i, c := range f.parent.children {
			if c == f {
				f.parent.children[i] = sibling
			}
		}
	}
	xproto.ReparentWindow(f.Conn(), sibling.window, sibling.parentWindow(), sibling.g.X, sibling.g.Y)
	sibling.layout()

	// the sibling is out of f's window: destroying f only takes f and child
	f.children = []*Frame{child}
	f.Destroy()

	ws.ActiveFrame = sibling.firstLeaf()
	ws.updateTitleBars()
	return nil
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
	if idx < f.activeClient {
		f.activeClient--
	}
	if f.activeClient >= len(f.clients) {
		f.activeClient = len(f.clients) - 1
	}
}
