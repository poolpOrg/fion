package main

import (
	"fmt"
	"log"
	"os"
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
	Frames map[xproto.Window]*FrameNode

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

func (wm *WM) ActiveScreen() *Screen {
	if len(wm.Screens) == 0 {
		return nil
	}
	return wm.Screens[0]
}

func (wm *WM) ActiveWorkspace() *Workspace {
	sc := wm.ActiveScreen()
	if sc == nil {
		return nil
	}
	return sc.ActiveWorkspace()
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
					txt := fmt.Sprintf("%s", time.Now().Format(time.RFC1123))
					// Clear the bar area (optional)
					xproto.PolyFillRectangle(wm.X, xproto.Drawable(ws.Bar), ws.BarGC,
						[]xproto.Rectangle{{X: 0, Y: 0, Width: 0, Height: 20}})
					xproto.ImageText8(wm.X, byte(len(txt)), xproto.Drawable(ws.Bar), ws.BarGC, 10, 13, txt)
					//						wm.drawWorkspaceLabel(ws

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
					if ev.Window == ws.Bar {
						txt := fmt.Sprintf("%s", time.Now().Format(time.RFC1123))
						// Clear the bar area (optional)
						xproto.PolyFillRectangle(wm.X, xproto.Drawable(ws.Bar), ws.BarGC,
							[]xproto.Rectangle{{X: 0, Y: 0, Width: 0, Height: 20}})
						xproto.ImageText8(wm.X, byte(len(txt)), xproto.Drawable(ws.Bar), ws.BarGC, 10, 13, txt)
						//						wm.drawWorkspaceLabel(ws
					}
				}
			}
		case xproto.MapRequestEvent:
			fmt.Printf("MapRequest: win=%d\n", ev.Window)
			wm.tryManage(ev.Window)
			xproto.MapWindow(wm.X, ev.Window)
		case xproto.ConfigureRequestEvent:
			wm.handleConfigure(ev)
		case xproto.DestroyNotifyEvent:
			wm.unmanageWindow(ev.Window)
		case xproto.UnmapNotifyEvent:
			wm.unmanageWindow(ev.Window)
		case xproto.KeyPressEvent:
			mods := ev.State & (xproto.ModMask1 | xproto.ModMask2 | xproto.ModMask3 | xproto.ModMask4 | xproto.ModMaskControl | xproto.ModMaskShift)

			log.Printf("KeyPressed: %d %d", ev.Detail, mods)
			if ev.Detail == Key_F9 && mods == xproto.ModMask2 {
				screen := wm.ActiveScreen()
				if screen == nil {
					continue
				}
				old := wm.ActiveScreen().ActiveWorkspace()
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
					wm.ActiveScreen().removeWorkspace()

				case Key_LeftArrow:
					old := wm.ActiveScreen().ActiveWorkspace()
					new := wm.ActiveScreen().cycleWorkspaceLeft()
					if old != nil && old != new {
						new.Map()
						old.Unmap()
					}

				case Key_RightArrow:
					old := wm.ActiveScreen().ActiveWorkspace()
					new := wm.ActiveScreen().cycleWorkspaceRight()
					if old != nil && old != new {
						new.Map()
						old.Unmap()
					}
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
			wm.handleClientMessage(ev)
		}
	}
}
