<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.png">
    <img alt="fion" src="assets/logo-light.png" width="420">
  </picture>
</p>

fion is a static tiling window manager for X11, inspired by
[ion](https://tuomov.iki.fi/software/ion/), written in Go on top of
[xgb](https://github.com/jezek/xgb).

It is young: it runs, but expect rough edges, and see the known
limitations below.

| | |
|:-:|:-:|
| ![frames and tabs](assets/screenshots/tabs.png) | ![the scratchpad](assets/screenshots/scratchpad.png) |
| frames split in tabs | the scratchpad, over them |
| ![the launcher](assets/screenshots/launcher.png) | ![the cheat sheet](assets/screenshots/cheatsheet.png) |
| the launcher | the cheat sheet, `Super+?` |
| ![the system panel](assets/screenshots/panel.png) | ![the CPU view](assets/screenshots/panel-cpu.png) |
| the system panel | its CPU view |


design
--
Each X screen holds one or more workspaces, and always shows one of them.
A workspace is a tree of frames: it starts as a single frame filling it,
and any frame can be split in two, stacked or side by side.
Leaf frames hold X clients as tabs: the frame's title bar has one tab per
client, and only the active one is shown.

Each screen also has a scratchpad, as in ion: a frame floating centered
above the workspaces, that `Super+space` shows and hides.  It stays shown
when switching workspaces.  While it is shown, it is the active frame: new
windows open in it, the tab bindings act on it and it has the keyboard
focus.  It can't be split or removed.  To fill it, show it and start
windows, `Super+t` for a terminal, or drag tabs onto it.

`Super+f` shows the active tab full screen, over the bar and the other
frames, and `Super+f` again puts it back in its frame; any other binding,
or a new window, puts it back first.

A bar at the bottom of each workspace shows the screen and workspace
numbers, the operating system with a small icon of its own, as
`OpenBSD/7.9 (arm64)`, CPU and memory usage, the load average and the
time.  The CPU use shows in bold red from 80%, the memory's from 90%, and
the load average above the number of CPUs.  On laptops it also shows the
battery's charge, its time left, or whether it is charging when plugged
in, in bold red from 15% on battery.  On narrower screens it shortens
what it shows to fit: a shorter clock, the percentages alone, the
system's icon without its name.

Clicking the bar, or `Super+s`, opens a panel above it, refreshed every
second, the frames shrinking to fit above it until it is closed.  Its
summary shows the machine's hardware, the use and temperature of each
CPU, the GPUs, the memory, the filesystems' usage, and every disk's and
network interface's throughput, idle ones marked rather than hidden, in
as many columns as the screen's width holds.  `Tab`, the arrows or `1` to
`6` switch to its CPU, Memory, Disk, Network and Sensors views, with
graphs of the time it has been shown, the last ten minutes at most;
`Escape` closes it.

Empty frames are black; a workspace that is a single empty frame shows the
fion logo, centered:

![an empty workspace](assets/screenshots/empty.png)

fion uses the [Dracula](https://draculatheme.com) colors, and adds them for
xterm to the display's resource database when it starts, along with UTF-8
whatever the locale, unless your own resources already set them.


building
--
fion needs Go 1.25 or later:

    $ go install github.com/poolpOrg/fion@latest

or, from a clone:

    $ go build


key bindings
--
All bindings use one modifier, Super by default, and `Super+?` shows them
in a cheat sheet:

| keys                            | action                                   |
|---------------------------------|------------------------------------------|
| `Super+Tab` / `Super+Shift+Tab` | next / previous tab                      |
| `Super+t`                       | new terminal tab                         |
| `Super+Left` `Right` `Up` `Down` | go to the frame on that side            |
| `Super+Shift+` an arrow         | split: new frame on that side            |
| `Super+Page Down` / `Page Up`   | next / previous workspace                |
| `Super+w`                       | new workspace                            |
| `Super+f`                       | show the active tab full screen / back   |
| `Super+d`, then `d`             | close what has the focus: the active window, asked to close first and killed when it didn't, an empty frame, or a workspace that is a single empty frame but for the last one. `Super+d` asks: `d` confirms, any other key cancels |
| `Super++` / `Super+-`           | resize the active frame, growing / shrinking: then the arrows move its edge on that side, `+` and `-` switch, `Return` confirms, `Escape` cancels |
| `Super+m`                       | move the scratchpad: then the arrows move it, `Return` confirms, `Escape` cancels; its place and size are kept for the next time |
| `Super+Return`                  | open the launcher                        |
| `Super+space`                   | show / hide the scratchpad               |
| `Super+s`                       | show / hide the system panel, as clicking the bar does |
| `Super+?`                       | this list                                |
| `Super+Escape`                  | quit fion                                |
| `Print`                         | capture: `s` a screenshot or `v` a video, then `t` the tab, `f` the frame or `w` the workspace; `Print` again stops a video |

Screenshots are saved as PNG, and videos, recorded with ffmpeg, as MP4, in
`$XDG_PICTURES_DIR`, or `~/Pictures`, or the home directory.  The bar
shows `REC` while recording.

Clicking a tab selects it and its frame.  Dragging a tab moves its window
to the frame it is dropped on, the scratchpad included, or to another place
in its own title bar.

The launcher finds, as you type, the commands in `$PATH`, the desktop
applications and the command lines you ran before, which come first.
Matching is fuzzy: `ffx` finds `firefox`.  `Up` / `Down` (or `Ctrl+p` /
`Ctrl+n`) move the selection, `Tab` copies it to the input line to add
arguments, `Return` runs the selection, or the line as typed when it has
arguments, through `sh -c`, and `Escape` closes the launcher.  Programs
that don't open windows, such as `ls` or `btop`, run in an xterm that stays
open once they exit, until `Return`: fion tells them from the libraries
they are linked to, and desktop applications from their entries.  The history
is kept in `$XDG_STATE_HOME/fion/history`, `~/.local/state/fion/history` by
default.

fion draws its text with the fixed font at a size for the screen: 13
pixels below 1000 lines, 15 up to 1400, 18 up to 1800, 20 above, and
the bars, tabs and panels grow with it, as does xterm's font.  The bar
at the bottom uses the next size up, 12x24 above 20.  Set `FION_FONT`
to 13, 15, 18 or 20 to pick a size, or to a core font's name, which the
bar uses too.

Set `FION_MODIFIER` to `ctrl`, `alt` or `mod1` to `mod5` to use another
modifier than Super.


running it nested
--
The easiest way to try fion is inside Xephyr:

    $ Xephyr :1 -screen 1280x800 &
    $ DISPLAY=:1 ./fion &
    $ DISPLAY=:1 xterm &

On macOS, with XQuartz, Xephyr can't create its socket in `/tmp/.X11-unix`
without root, and XQuartz has no Super key, so listen on TCP and use Ctrl:

    $ Xephyr :1 -screen 1280x800 -nolisten unix -nolisten local -listen tcp -ac &
    $ DISPLAY=127.0.0.1:1 FION_MODIFIER=ctrl ./fion


tests
--
The frame tree tests need an X server fion can manage, such as Xvfb, named
by `FION_TEST_DISPLAY`; they are skipped otherwise:

    $ Xvfb :99 -noreset &
    $ FION_TEST_DISPLAY=:99 go test ./wm


known limitations
--
- input focus is only given to the scratchpad and to a tab shown full
  screen; elsewhere it follows the pointer
- windows can only be moved between frames with the mouse
- every window gets a tab, dialogs and transient windows included
- ConfigureRequest events are ignored, and clients can't ask to be shown
  full screen themselves
- only the X screens are handled, not RandR outputs: on a multi-monitor
  setup a workspace spans all the monitors
- tab titles are read from `WM_NAME` only, UTF-8 titles show empty
- EWMH support is limited to announcing the window manager
