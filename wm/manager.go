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
	xConn *xgb.Conn
	Setup *xproto.SetupInfo

	KeyboardManager *KeyboardManager

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
	wm.KeyboardManager = NewKeyboardManager(wm)

	if err := wm.initScreens(); err != nil {
		wm.Close()
		return nil, err
	}

	return wm, nil
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

const (
	M_Workspace = 1
	M_Frame     = 2
)

func (wm *Manager) Run() error {

	base := wm.KeyboardManager.Super
	for _, scr := range wm.Screens {
		_ = wm.KeyboardManager.GrabNamed(scr.Info().Root, "F9", base)
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

	mode := 0
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
			km := wm.KeyboardManager

			mods := ev.State &^ (xproto.ModMaskLock | km.Num)
			sym := km.eventKeysym(ev.Detail, ev.State)

			log.Printf("KeyPressed: %d %x %d", ev.Detail, sym, mods)

			if mods == km.Super && sym == XK_w {
				mode = M_Workspace
				continue
			}
			if mods == km.Super && sym == XK_f {
				mode = M_Frame
				continue
			}
			if mods == km.Super && sym == XK_Escape {
				return nil
			}

			if mods != 0 {
				mode = 0
			}

			if mode == 0 {
				switch sym {
				case XK_Space:
					fmt.Println("TODO: SCRATCHPAD")
				case XK_F1:
					fmt.Println("TODO: browser")
				case XK_F2:
					exec.Command("xterm", "-bg", "black", "-fg", "white").Start()
				}
			}

			if mode == M_Workspace {
				switch sym {
				case XK_c:
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

				case XK_d:
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
				case XK_h:
					wm.GetActiveWorkspace().splitH()

				case XK_v:
					wm.GetActiveWorkspace().splitV()

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

			if mode == M_Frame {
				switch sym {
				case XK_p:
					fmt.Println("previous workspace")
					wm.GetActiveWorkspace().cycleFrameLeft()
					for _, s := range wm.Screens {
						for _, ws := range s.Workspaces {
							ws.updateTitleBars()
						}
					}

				case XK_n:
					fmt.Println("nextworkspace")
					wm.GetActiveWorkspace().cycleFrameRight()
					for _, s := range wm.Screens {
						for _, ws := range s.Workspaces {
							ws.updateTitleBars()
						}
					}
				}
			}

			mode = 0

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

type Geometry struct {
	X, Y int16
	W, H uint16
}
