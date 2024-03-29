# fion
repository for the fion window manager

**THIS IS A WORK IN PROGRESS, IT IS NOT WORKING YET !**

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
