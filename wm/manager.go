package wm

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

type Manager struct {
	xConn *xgb.Conn
	Setup *xproto.SetupInfo

	KeyboardManager *KeyboardManager

	Screens []*Screen

	NumLock uint16

	// Clients by window id
	Clients map[xproto.Window]*Client
	Frames  map[xproto.Window]*Frame

	// pending key prefix (M_Workspace, M_Frame, M_Client), 0 when none
	mode int

	// tab being dragged, nil when none
	drag *tabDrag
}

func NewManager() (*Manager, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}

	setup := xproto.Setup(conn)
	if setup == nil || len(setup.Roots) == 0 {
		conn.Close()
		return nil, fmt.Errorf("no X screens found")
	}

	wm := &Manager{
		xConn: conn,
		Setup: setup,

		//Atoms:   getAtoms(conn),
		Frames:  make(map[xproto.Window]*Frame),
		Clients: make(map[xproto.Window]*Client),
	}
	wm.KeyboardManager = NewKeyboardManager(wm)

	if err := wm.initScreens(); err != nil {
		wm.Close()
		return nil, err
	}
	wm.installXtermTheme()
	wm.adoptExisting()

	return wm, nil
}

// adoptExisting manages the windows that were shown before fion started,
// bottom to top, so that the topmost one ends up the active tab. Those
// that are unmapped will be managed when they are mapped.
func (wm *Manager) adoptExisting() {
	// frames are all on the active screen for now
	s := wm.GetActiveScreen()
	for _, win := range s.existing {
		attr, err := xproto.GetWindowAttributes(wm.Conn(), win).Reply()
		if err != nil || attr.OverrideRedirect || attr.MapState != xproto.MapStateViewable {
			continue
		}
		wm.manageWindow(win, true)
	}
	s.existing = nil
}

func (wm *Manager) tryManage(w xproto.Window) {
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
	wm.manageWindow(w, false)
}

// manageWindow makes win a tab of the active frame. mapped tells whether
// the window is already shown, as when adopting it at startup.
func (wm *Manager) manageWindow(win xproto.Window, mapped bool) {
	if _, exists := wm.Clients[win]; exists {
		return
	}

	// Make a tab for client
	bw := uint32(1)

	frame := wm.GetActiveFrame()
	parentId := frame.GetWindow()

	geom, err := xproto.GetGeometry(wm.Conn(), xproto.Drawable(parentId)).Reply()
	if err != nil {
		return
	}
	fmt.Println("Parent geom:", geom, geom.Width, geom.Height, geom.X, geom.Y)

	// Once reparented the client is no longer a child of the root, so the
	// root's SubstructureNotify stops reporting on it: watch it directly,
	// along with its properties for the tab title.
	xproto.ChangeWindowAttributes(wm.Conn(), win, xproto.CwEventMask,
		[]uint32{xproto.EventMaskStructureNotify | xproto.EventMaskPropertyChange})
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

	// mapped by the frame, as its new active tab
	c := &Client{frame: frame, mapped: mapped}
	if mapped {
		// reparenting a mapped window unmaps it first
		c.ignoreUnmap++
	}
	wm.Clients[win] = c
	frame.AddTab(win)
	log.Printf("managing 0x%x", win)

	// Attach to active Leaf as a new tab
	//wm.updateClientList()
	// wm.layoutWorkspace(ws)
}

// forgetClient drops a client from fion's bookkeeping without touching the
// window itself, and returns what was known about it.
func (wm *Manager) forgetClient(win xproto.Window) *Client {
	c, ok := wm.Clients[win]
	if !ok {
		return nil
	}
	delete(wm.Clients, win)
	c.frame.RemoveClient(win)
	c.frame.showActiveClient()
	c.frame.updateTitleBar()
	if c.frame.floating() {
		wm.updateFocus()
	}
	return c
}

// unmanageWindow destroys a client window at the user's request.
func (wm *Manager) unmanageWindow(win xproto.Window) {
	if wm.forgetClient(win) == nil {
		return
	}
	xproto.DestroyWindow(wm.Conn(), win)
}

func (wm *Manager) handleDestroyNotify(ev xproto.DestroyNotifyEvent) {
	if wm.forgetClient(ev.Window) != nil {
		log.Printf("0x%x destroyed", ev.Window)
	}
}

func (wm *Manager) handleUnmapNotify(ev xproto.UnmapNotifyEvent) {
	// A window that is a child of the root, as when it is adopted, is also
	// reported to the root: only count the report on the window itself.
	if ev.Event != ev.Window {
		return
	}
	c, ok := wm.Clients[ev.Window]
	if !ok {
		return
	}
	if c.ignoreUnmap > 0 {
		c.ignoreUnmap--
		return
	}

	// The client withdrew its window. Hand it back to the root so that a
	// later MapRequest manages it afresh.
	wm.forgetClient(ev.Window)
	log.Printf("0x%x withdrawn", ev.Window)

	// A client that exits unmaps its window just before destroying it, so
	// the window is often gone already: check these requests and drop the
	// errors rather than have them reported as X errors by the event loop.
	root := c.frame.screen.Info().Root
	_ = xproto.ChangeWindowAttributesChecked(wm.Conn(), ev.Window, xproto.CwEventMask, []uint32{xproto.EventMaskNoEvent}).Check()
	_ = xproto.ReparentWindowChecked(wm.Conn(), ev.Window, root, 0, 0).Check()
	_ = xproto.ChangeSaveSetChecked(wm.Conn(), xproto.SetModeDelete, ev.Window).Check()
}

//func (wm *WM) updateClientList() {
//	buf := make([]byte, 0, 4*len(wm.Clients))
//	for w := range wm.Clients {
//		buf = append(buf, byte(w), byte(w>>8), byte(w>>16), byte(w>>24))
//	}
//	xproto.ChangeProperty(wm.X, xproto.PropModeReplace, wm.Root, wm.Atoms.NET_CLIENT_LIST, xproto.AtomWindow, 32, uint32(len(buf)/4), buf)
//}

func (wm *Manager) Conn() *xgb.Conn {
	return wm.xConn
}

func (wm *Manager) Close() {
	if wm.xConn != nil {
		wm.xConn.Close()
	}
}

func (wm *Manager) initScreens() error {
	for _, scr := range xproto.Setup(wm.xConn).Roots {
		screen, err := newScreen(wm, scr)
		if err != nil {
			return err
		}
		wm.Screens = append(wm.Screens, screen)
	}
	return nil
}

func (wm *Manager) GetActiveScreen() *Screen {
	if len(wm.Screens) == 0 {
		return nil
	}
	return wm.Screens[0]
}

func (wm *Manager) GetActiveWorkspace() *Workspace {
	sc := wm.GetActiveScreen()
	if sc == nil {
		return nil
	}
	return sc.GetActiveWorkspace()
}

func (wm *Manager) GetActiveFrame() *Frame {
	sc := wm.GetActiveScreen()
	if sc == nil {
		return nil
	}
	if sc.scratchpadShown {
		return sc.scratchpad
	}
	return sc.GetActiveWorkspace().GetActiveFrame()
}

func (wm *Manager) GetActiveClient() xproto.Window {
	return wm.GetActiveFrame().GetActiveClient()
}

const (
	M_Workspace = 1
	M_Frame     = 2
	M_Client    = 3 // tabs of the active frame
)

// xEvent is what the X connection hands us: an event or an error, never both.
type xEvent struct {
	event xgb.Event
	err   xgb.Error
}

// pumpEvents forwards X events to a channel so that Run can select on them
// alongside timers and signals. The channel is closed when the X connection
// goes away.
func (wm *Manager) pumpEvents(done <-chan struct{}) <-chan xEvent {
	events := make(chan xEvent)
	go func() {
		defer close(events)
		for {
			ev, err := wm.xConn.WaitForEvent()
			if ev == nil && err == nil {
				return
			}
			select {
			case events <- xEvent{event: ev, err: err}:
			case <-done:
				return
			}
		}
	}()
	return events
}

// spawn starts a program and reaps it when it exits.
func (wm *Manager) spawn(name string, args ...string) {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		log.Printf("spawn %s: %v", name, err)
		return
	}
	go cmd.Wait()
}

// spawnTerminal starts an xterm, which opens in the active frame, in the
// colors installXtermTheme set up.
func (wm *Manager) spawnTerminal() {
	wm.spawn("xterm")
}

// Run is the event loop. All state changes and all drawing happen on this
// goroutine, so the workspace and frame trees need no locking.
func (wm *Manager) Run() error {

	wm.KeyboardManager.GrabBindings()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	done := make(chan struct{})
	defer close(done)
	events := wm.pumpEvents(done)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	keymap := time.NewTicker(250 * time.Millisecond)
	defer keymap.Stop()

	log.Printf("fion running on %q — %s+Escape quits", os.Getenv("DISPLAY"), wm.KeyboardManager.ModName)

	for {
		select {
		case sig := <-signals:
			log.Printf("received %v, exiting", sig)
			return nil

		case <-ticker.C:
			for _, s := range wm.Screens {
				for _, ws := range s.Workspaces {
					ws.updateInfoBar()
				}
				s.updateTitleBars()
			}

		case <-keymap.C:
			if wm.KeyboardManager.CheckMapping() {
				log.Printf("keyboard mapping changed, bindings grabbed again")
			}

		case ev, ok := <-events:
			if !ok {
				return fmt.Errorf("X connection closed")
			}
			// Errors from unchecked requests land here, typically BadWindow
			// for a client that went away before we caught up. They are
			// not fatal to the window manager.
			if ev.err != nil {
				log.Printf("X error: %v", ev.err)
				continue
			}
			if quit := wm.handleEvent(ev.event); quit {
				return nil
			}
		}
	}
}

// handleEvent dispatches one X event and reports whether the user asked to quit.
func (wm *Manager) handleEvent(e xgb.Event) bool {
	switch ev := e.(type) {
	case xproto.ExposeEvent:
		if wm.drag != nil && ev.Window == wm.drag.window {
			wm.drawDragWindow()
			break
		}
		// If the expose is for a workspace bar, redraw its label
		for _, s := range wm.Screens {
			for _, ws := range s.Workspaces {
				if ev.Window == ws.InfoBarWindow {
					ws.updateInfoBar()
				}
			}
			s.updateTitleBars()
		}
	case xproto.MapRequestEvent:
		wm.tryManage(ev.Window)
	case xproto.ConfigureRequestEvent:
		//wm.handleConfigure(ev)
	case xproto.DestroyNotifyEvent:
		wm.handleDestroyNotify(ev)
	case xproto.UnmapNotifyEvent:
		wm.handleUnmapNotify(ev)
	case xproto.KeyPressEvent:
		return wm.handleKeyPress(ev)
	case xproto.MappingNotifyEvent:
		if ev.Request != xproto.MappingPointer {
			wm.KeyboardManager.MappingChanged()
		}
	case xproto.PropertyNotifyEvent:
		if ev.Atom == xproto.AtomWmName {
			if c, ok := wm.Clients[ev.Window]; ok {
				c.frame.updateTitleBar()
			}
		}
	case xproto.ButtonPressEvent:
		if f, ok := wm.Frames[ev.Event]; ok && ev.Detail == 1 {
			wm.clickTitleBar(f, ev.EventX)
			wm.beginTabDrag(f, ev)
		}
	case xproto.MotionNotifyEvent:
		wm.dragMotion(ev)
	case xproto.ButtonReleaseEvent:
		wm.endTabDrag(ev)
		/*z
		// Focus on click; Alt+Left move, Alt+Right resize on frame
		if leaf := wm.Frames[ev.Event]; leaf != nil {
			wm.ActiveWorkspace().FocusedFrame = leaf
		}
		if ev.State&xproto.ModMask1 != 0 {
			if ev.Detail == 1 {
				if cl := wm.clientByFrame(ev.Event); cl != nil {
					wm.beginDragMove(cl, ev)
				}
			}
			if ev.Detail == 3 {
				if cl := wm.clientByFrame(ev.Event); cl != nil {
					wm.beginDragResize(cl, ev)
				}
			}
		}
		for si, s := range wm.Screens {
			for wi, ws := range s.Workspaces {
				if ev.Event == ws.Bar {
					dprintf("bar click screen=%d ws=%d button=%d at (%d,%d)", si, wi, ev.Detail, ev.EventX, ev.EventY)
					// Evxample: left-click cycles tabs
					if ev.Detail == 1 {
						wm.FocusNextTab()
					}
				}
			}
		}
		*/
	case xproto.ClientMessageEvent:
		//wm.handleClientMessage(ev)
	}
	return false
}

// clickTitleBar makes f the active frame and selects the tab at x.
func (wm *Manager) clickTitleBar(f *Frame, x int16) {
	if !f.floating() {
		f.workspace.ActiveFrame = f
	}
	if i := f.tabAt(x); i >= 0 {
		f.selectClient(i)
	}
	f.screen.updateTitleBars()
}

// closeActive closes the active client or, in an empty frame, removes the
// frame, or in the last frame of a workspace, the workspace.
func (wm *Manager) closeActive() error {
	frame := wm.GetActiveFrame()
	if client := frame.GetActiveClient(); client != 0 {
		wm.unmanageWindow(client)
		return nil
	}
	switch {
	case frame.floating():
		return fmt.Errorf("the scratchpad can't be removed")
	case frame.parent != nil:
		return frame.parent.RemoveChild(frame)
	}
	wm.GetActiveScreen().removeWorkspace()
	return nil
}

// handleKeyPress implements the prefix bindings (Mod+w, Mod+f, Mod+t) and
// reports whether the user asked to quit.
//
// The prefixes and Mod+Escape are grabbed on the root, so they reach fion
// wherever the pointer is. A prefix then grabs the whole keyboard until the
// key that completes it, so that key doesn't go to the client under the
// pointer either.
func (wm *Manager) handleKeyPress(ev xproto.KeyPressEvent) bool {
	km := wm.KeyboardManager

	mods := ev.State &^ (xproto.ModMaskLock | km.Num)
	sym := km.eventKeysym(ev.Detail, ev.State)

	log.Printf("KeyPressed: %d %x %d", ev.Detail, sym, mods)

	if mods == km.Mod && (sym == XK_w || sym == XK_f || sym == XK_t) {
		switch sym {
		case XK_w:
			wm.mode = M_Workspace
		case XK_f:
			wm.mode = M_Frame
		case XK_t:
			wm.mode = M_Client
		}
		if err := km.GrabKeyboard(ev.Root); err != nil {
			log.Printf("grab keyboard: %v", err)
		}
		return false
	}
	if mods == km.Mod && sym == XK_Escape {
		return true
	}
	if mods == km.Mod && sym == XK_F2 {
		wm.spawnTerminal()
		return false
	}
	if mods == km.Mod && sym == XK_Space {
		if err := wm.GetActiveScreen().toggleScratchpad(); err != nil {
			log.Printf("scratchpad: %v", err)
		}
		return false
	}

	// pressing Shift or releasing Mod and pressing it again doesn't
	// complete a prefix
	if wm.mode != 0 && isModifierKey(sym) {
		return false
	}

	mode := wm.mode
	wm.mode = 0
	if mode != 0 {
		km.UngrabKeyboard()
	}
	if mods != 0 {
		mode = 0
	}

	if mode == 0 {
		switch sym {
		case XK_F1:
			fmt.Println("TODO: browser")
		case XK_F2:
			wm.spawnTerminal()
		}
	}

	if mode == M_Workspace {
		switch sym {
		case XK_c:
			screen := wm.GetActiveScreen()
			if screen == nil {
				return false
			}
			old := wm.GetActiveWorkspace()
			ws, err := screen.newWorkspace()
			if err != nil {
				log.Printf("newWorkspace: %v", err)
				return false
			}
			ws.Map()
			old.Unmap()

		case XK_d:
			if err := wm.closeActive(); err != nil {
				log.Printf("close: %v", err)
			}
		case XK_h:
			if err := wm.GetActiveFrame().splitH(); err != nil {
				log.Printf("split: %v", err)
			}

		case XK_v:
			if err := wm.GetActiveFrame().splitV(); err != nil {
				log.Printf("split: %v", err)
			}

		case XK_p:
			old := wm.GetActiveWorkspace()
			new := wm.GetActiveScreen().cycleWorkspaceLeft()
			if old != nil && old != new {
				new.Map()
				old.Unmap()
			}

		case XK_n:
			old := wm.GetActiveWorkspace()
			new := wm.GetActiveScreen().cycleWorkspaceRight()
			if old != nil && old != new {
				new.Map()
				old.Unmap()
			}
		}
	}

	// the tiled frames are behind the scratchpad while it is shown
	if mode == M_Frame && wm.GetActiveScreen().scratchpadShown {
		log.Printf("the scratchpad is shown, frame cycling ignored")
		mode = 0
	}
	if mode == M_Frame {
		switch sym {
		case XK_p:
			fmt.Println("previous workspace")
			wm.GetActiveWorkspace().cycleFrameLeft()
			wm.GetActiveScreen().updateTitleBars()

		case XK_n:
			fmt.Println("nextworkspace")
			wm.GetActiveWorkspace().cycleFrameRight()
			wm.GetActiveScreen().updateTitleBars()
		}
	}

	if mode == M_Client {
		switch sym {
		case XK_n:
			wm.GetActiveFrame().cycleClientRight()
		case XK_p:
			wm.GetActiveFrame().cycleClientLeft()
		}
	}

	/*



			case XK_Up:
				wm.GetActiveFrame().cycleClientLeft()
				for _, s := range wm.Screens {
					for _, ws := range s.Workspaces {
						ws.updateTitleBars()
					}
				}

			case XK_Down:
				wm.GetActiveFrame().cycleClientRight()
				for _, s := range wm.Screens {
					for _, ws := range s.Workspaces {
						ws.updateTitleBars()
					}
				}

			}
		}
	*/

	/*
			case km.Keycode("LeftArrow"):
				old := wm.GetActiveWorkspace()
				new := wm.GetActiveScreen().cycleWorkspaceLeft()
				if old != nil && old != new {
					new.Map()
					old.Unmap()
				}

			case km.Keycode("RightArrow"):
				old := wm.GetActiveWorkspace()
				new := wm.GetActiveScreen().cycleWorkspaceRight()
				if old != nil && old != new {
					new.Map()
					old.Unmap()
				}

		}

	*/

	return false
}

type Geometry struct {
	X, Y int16
	W, H uint16
}
