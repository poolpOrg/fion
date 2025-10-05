package main

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

type WM struct {
	X *xgb.Conn

	Setup   *xproto.SetupInfo
	Screens []*Screen

	Scr   *xproto.ScreenInfo
	Root  xproto.Window
	Atoms Atoms

	NumLock uint16

	SupportingWin xproto.Window

	// Clients by window id
	Clients map[xproto.Window]*Client

	// Frame lookup by window id (frame -> leaf)
	Frames map[xproto.Window]*Frame

	Focused *Client

	ColFocused uint32 // pixel
	ColNormal  uint32 // pixelq1

}

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
		Frames:  make(map[xproto.Window]*Frame),
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

	//setDefaultCursor(conn, wm.Root)

	if err := wm.initEWMH(); err != nil {
		return nil, err
	}

	wm.initScreens()
	wm.manageExistingWindows()

	return wm, nil
}

func (wm *WM) Close() {
	if wm.X != nil {
		wm.X.Close()
	}
}

func (wm *WM) initScreens() error {
	if err := randr.Init(wm.X); err != nil {
		return err
	}
	for _, scr := range xproto.Setup(wm.X).Roots {
		screen, err := newScreen(wm, scr)
		if err != nil {
			return err
		}
		wm.Screens = append(wm.Screens, screen)
	}
	return nil
}

func (wm *WM) GetActiveScreen() *Screen {
	if len(wm.Screens) == 0 {
		return nil
	}
	return wm.Screens[0]
}

func (wm *WM) GetActiveWorkspace() *Workspace {
	sc := wm.GetActiveScreen()
	if sc == nil {
		return nil
	}
	return sc.GetActiveWorkspace()
}

func (wm *WM) GetActiveFrame() *Frame {
	sc := wm.GetActiveScreen()
	if sc == nil {
		return nil
	}
	return sc.GetActiveWorkspace().GetActiveFrame()
}

func (wm *WM) Run() error {
	// Keybinding: Alt+Q to quit
	_ = wm.grabKey(wm.Root, xproto.ModMask1, KeyQ)

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
					for _, f := range ws.Root.Children {
						f.updateTitleBar()
					}
				}
			}
			time.Sleep(1 * time.Second)
		}
	}()

	for {
		e, err := wm.X.WaitForEvent()
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

			if mods == xproto.ModMaskShift {
				switch ev.Detail {
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

			if mods == xproto.ModMask2 {
				switch ev.Detail {
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

				case Key_UpArrow:
					wm.GetActiveWorkspace().splitV()

				case Key_DownArrow:
					wm.GetActiveWorkspace().splitH()

				}
			}

			if mods == 0 {
				switch ev.Detail {
				case Key_F2:
					exec.Command("xterm", "-bg", "black", "-fg", "white").Start()
				}
			}

		case xproto.ButtonPressEvent:
			/*
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
