package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

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

func (wm *WM) initScreens() error {
	if err := randr.Init(wm.X); err != nil {
		return err
	}
	for si, scr := range xproto.Setup(wm.X).Roots {
		screen, err := newScreen(wm, si, scr)
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
						//						wm.drawWorkspaceLabel(ws)
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
				ws, err := screen.newWorkspace()
				if err != nil {
					log.Printf("newWorkspace: %v", err)
					continue
				}
				_ = ws
				log.Printf("CREATE NEW WORKSPACE: %d %d", ev.Detail, mods)
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
