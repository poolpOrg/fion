# fion
repository for the fion window manager

**THIS IS A WORK IN PROGRESS, IT IS NOT WORKING YET !**

```
Screen
 ├─ Frame (split H)
 │   ├─ Frame (split V)
 │   │   ├─ Tabbed group (client A, B)
 │   │   └─ Tabbed group (client C)
 │   └─ Tabbed group (client D)


Top-priority (must fix to be usable daily)
ICCCM basics
Handle/advertise WM_PROTOCOLS (WM_DELETE_WINDOW, WM_TAKE_FOCUS).
Respect WM_NORMAL_HINTS (size increment, min/max, aspect).
Ignore/manage windows with override_redirect = true (tooltips/menus). Right now you treat everything the same.
EWMH/NetWM support
Set _NET_SUPPORTING_WM_CHECK and _NET_SUPPORTED.
Maintain _NET_CLIENT_LIST, _NET_ACTIVE_WINDOW.
Implement _NET_WM_STATE (at least: FULLSCREEN, MAXIMIZED_VERT/HORZ, HIDDEN), _NET_NUMBER_OF_DESKTOPS, _NET_CURRENT_DESKTOP, _NET_WORKAREA.
Respond to _NET_ACTIVE_WINDOW, _NET_CLOSE_WINDOW, _NET_MOVERESIZE_WINDOW.
Focus policy (real)
Implement click-to-focus or sloppy-focus properly:
Track EnterNotify/LeaveNotify, FocusIn/FocusOut.
Honor WM_HINTS input model and WM_TAKE_FOCUS.
Don’t always SetInputFocus on MapRequest—that steals focus unexpectedly.
Reparenting & decorations
Create a frame window per client (titlebar/borders).
Reparent client into frame on manage; map/unmap both correctly.
On ConfigureRequest of a managed client, do not forward geometry directly to the client—resize/move the frame and send ConfigureNotify to the client with adjusted coords.
Correct (Un)map/Destroy sequencing
Distinguish client-initiated UnmapNotify vs WM-initiated unmap (ICCCM Withdrawn/Normal/Zoomed states).
Clean up per-client state on DestroyNotify.
Use SaveSet if you reparent, to ensure clients are remapped if WM exits.
Key handling (robust)
Don’t rely on a hardcoded keycode. Translate KeySyms → Keycodes (XKB or GetKeyboardMapping).
Grab each binding with all lock modifiers masked (NumLock, CapsLock, ScrollLock), otherwise shortcuts fail half the time.
Multi-screen/outputs
Track screen size/monitors. Use RandR to handle resolution and output changes (workareas per monitor).
Place/manage windows per monitor, not just a single root geometry.
Important next steps (quality-of-life)
Event selection & errors
Select more events on clients/frames (e.g., EnterWindow, FocusChange, PropertyChange).
Set an X error handler and handle BadWindow/BadMatch gracefully (races are common when clients exit).
Move/resize interactions
Mouse binds on frame edges/titlebar to move/resize (with ButtonPress/Release/MotionNotify grabs).
Implement EWMH-compliant _NET_WM_MOVERESIZE.
Window types & transient windows
Read _NET_WM_WINDOW_TYPE to treat dialogs, menus, docks, splash appropriately.
Center or attach transients (WM_TRANSIENT_FOR) over their parents and keep them above.
Fullscreen, maximized, struts
Respect panel/dock struts (_NET_WM_STRUT_PARTIAL) for workarea computations.
Handle fullscreen/restore transitions cleanly.
State persistence & restarts
Re-manage on restart (set a unique selection like WM_S0; detect if another WM owns it).
Optional: simple session restore of placements.
Performance & correctness nits
Don’t spam MapWindow on already-viewable clients.
Batch property/atom queries; cache Atoms.
Avoid blocking behaviors in the event loop (long operations should be async).


Minimal API surface to add (clean skeleton structure)

atoms.go: intern all ICCCM/EWMH atoms once.
client.go: struct tracking window, frame, states, hints.
manage.go: manage/unmanage, reparent, decorations, hints read.
ewmh.go: set _NET_*, handle client messages.
focus.go: focus policy, active window bookkeeping.
keys.go: keysym→keycode map, grabs with lock masks.
randr.go: screen/monitor changes, workareas.
mouse.go: move/resize logic.
```


![fion](https://poolp.org/posts/2019-08-25/august-2019-report-fion-plakar-and-opensmtpd/cover.jpg)

description
--
fion is a static tiling window manager inspired by ion.


design
--
Fion assigns a work area to each screen.
Each work area manages one or many workspaces and will always display an active workspace at a given time.
A workspace will always contain at least one tile filling it up entirely.
Tiles may be split horizontally or vertically.


currently implemented
--
- detects and configures multiple screens
- assigns a workarea, default workspace and default tile to each screen
- as many workspaces as wanted on each screen
- as many tiles as wanted on each workspace
- keyboard shortcuts to create / destroy / switch between next and previous workspace
- keyboard shortcuts to split horizontally & vertically / destroy / switch between next and previous tile
- keyboard shortcut to run terminal
- notion of current workspace and current tile on each screen
- attaches X client to the proper place
- focus is given to a tile either through keyboard shortcuts or by moving cursor
- event loop implements a tick to update layout even in the lack of events


missing
--
- window management should work when focus is on a terminal, hijacking key strokes
- tiles management is not finished: creating / splitting / iterating works fine but destroying breaks the layout
- framing inside tiles so that it is possible to iterate between X clients attached to the same tile
- splitting tiles halves the parent tile, support for resizing should be implemented
- a cross workspace tile should be implemented, similar to ion's alt-space tile


obligatory screenshots
--
![1](https://poolp.org/images/2019-06-30-fion_1.png)
![2](https://poolp.org/images/2019-06-30-fion_2.png)
