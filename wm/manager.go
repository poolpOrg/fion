package wm

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/randr"
	"github.com/BurntSushi/xgb/xproto"
)

type Manager struct {
	xConn   *xgb.Conn
	Setup   *xproto.SetupInfo
	Screens []*Screen

	NumLock uint16

	// Clients by window id
	Clients map[xproto.Window]struct{}
	Frames  map[xproto.Window]*Frame
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
		Clients: make(map[xproto.Window]struct{}),
	}

	if err := wm.initScreens(); err != nil {
		wm.Close()
		return nil, err
	}

	return wm, nil
}

type Geometry struct {
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
	wm.manageWindow(w)
}

func (wm *Manager) manageWindow(win xproto.Window) {
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

func (wm *Manager) unmanageWindow(win xproto.Window) {
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

func (wm *Manager) Conn() *xgb.Conn {
	return wm.xConn
}

func (wm *Manager) Close() {
	if wm.xConn != nil {
		wm.xConn.Close()
	}
}

func (wm *Manager) initScreens() error {
	if err := randr.Init(wm.xConn); err != nil {
		return err
	}
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
	return sc.GetActiveWorkspace().GetActiveFrame()
}

func (wm *Manager) GetActiveClient() xproto.Window {
	return wm.GetActiveFrame().GetActiveClient()
}

func (wm *Manager) Run() error {
	for _, scr := range wm.Screens {
		_ = wm.grabKey(scr.Info().Root, xproto.ModMask4, Key_BackQuote)
		_ = wm.grabKey(scr.Info().Root, xproto.ModMask1, Key_D)
		_ = wm.grabKey(scr.Info().Root, xproto.ModMask1, Key_Q)
	}

	// Signals
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-done; os.Exit(0) }()
	log.Printf("fion running on %q — Alt+Q quits", os.Getenv("DISPLAY"))

	go func() {
		for {
			// If the expose is for a workspace bar, redraw its label
			for _, s := range wm.Screens {
				for _, ws := range s.Workspaces {
					ws.updateInfoBar()
					ws.updateTitleBars()
				}
			}
			time.Sleep(1 * time.Second)
		}
	}()

	for {
		e, err := wm.xConn.WaitForEvent()
		if err != nil {
			return fmt.Errorf("WaitForEvent: %w", err)
		}
		switch ev := e.(type) {
		case xproto.ExposeEvent:
			// If the expose is for a workspace bar, redraw its label
			for _, s := range wm.Screens {
				for _, ws := range s.Workspaces {
					if ev.Window == ws.InfoBarWindow {
						ws.updateInfoBar()
					}
					ws.updateTitleBars()
				}
			}
		case xproto.MapRequestEvent:
			wm.tryManage(ev.Window)
		case xproto.ConfigureRequestEvent:
			//wm.handleConfigure(ev)
		case xproto.DestroyNotifyEvent:
			//wm.unmanageWindow(ev.Window)
		case xproto.UnmapNotifyEvent:
			//wm.unmanageWindow(ev.Window)
		case xproto.KeyPressEvent:
			mods := ev.State & (xproto.ModMask1 | xproto.ModMask2 | xproto.ModMask3 | xproto.ModMask4 | xproto.ModMaskControl | xproto.ModMaskShift)

			log.Printf("KeyPressed: %d %d", ev.Detail, mods)
			if ev.Detail == Key_F9 && mods == xproto.ModMask2 {
				screen := wm.GetActiveScreen()
				if screen == nil {
					continue
				}
				old := wm.GetActiveWorkspace()
				ws, err := screen.newWorkspace()
				if err != nil {
					log.Printf("newWorkspace: %v", err)
					continue
				}
				ws.Map()
				old.Unmap()
			}

			if mods == xproto.ModMask2 {
				switch ev.Detail {
				case Key_D:
					frame := wm.GetActiveFrame()
					client := wm.GetActiveClient()
					if client != 0 {
						frame.RemoveClient(client)
						wm.unmanageWindow(client)
					} else if frame.GetParent() != nil {
						parent := frame.GetParent()
						parent.RemoveChild(frame)
					} else {
						wm.GetActiveScreen().removeWorkspace()
					}

					//frame := wm.GetActiveFrame()

				//	wm.GetActiveFrame().removeActiveClient()
				case Key_H:
					wm.GetActiveWorkspace().splitH()
				case Key_V:
					wm.GetActiveWorkspace().splitV()

				case Key_LeftArrow:
					wm.GetActiveWorkspace().cycleFrameLeft()
					for _, s := range wm.Screens {
						for _, ws := range s.Workspaces {
							ws.updateTitleBars()
						}
					}
				case Key_RightArrow:
					wm.GetActiveWorkspace().cycleFrameRight()
					for _, s := range wm.Screens {
						for _, ws := range s.Workspaces {
							ws.updateTitleBars()
						}
					}
				case Key_UpArrow:
					wm.GetActiveFrame().cycleClientLeft()
					for _, s := range wm.Screens {
						for _, ws := range s.Workspaces {
							ws.updateTitleBars()
						}
					}
				case Key_DownArrow:
					wm.GetActiveFrame().cycleClientRight()
					for _, s := range wm.Screens {
						for _, ws := range s.Workspaces {
							ws.updateTitleBars()
						}
					}
				}
			}

			if mods == xproto.ModMask2|xproto.ModMaskShift {
				switch ev.Detail {
				case Key_W:
					screen := wm.GetActiveScreen()
					if screen == nil {
						continue
					}
					old := wm.GetActiveWorkspace()
					ws, err := screen.newWorkspace()
					if err != nil {
						log.Printf("newWorkspace: %v", err)
						continue
					}
					ws.Map()
					old.Unmap()

				case Key_D:
					wm.GetActiveScreen().removeWorkspace()

				case Key_LeftArrow:
					old := wm.GetActiveWorkspace()
					new := wm.GetActiveScreen().cycleWorkspaceLeft()
					if old != nil && old != new {
						new.Map()
						old.Unmap()
					}

				case Key_RightArrow:
					old := wm.GetActiveWorkspace()
					new := wm.GetActiveScreen().cycleWorkspaceRight()
					if old != nil && old != new {
						new.Map()
						old.Unmap()
					}

				case Key_Space:
					fmt.Println("TODO: SCRATCHPAD")
				}
			}

			if mods == 0 {
				switch ev.Detail {
				case Key_F1:
					fmt.Println("TODO: browser")

				case Key_F2:
					exec.Command("xterm", "-bg", "black", "-fg", "white").Start()

				}
			}

		case xproto.ButtonPressEvent:
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
	}
}
