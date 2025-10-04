/*
 * Copyright (c) 2019 Gilles Chehade <gilles@poolp.org>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

#include <xcb/xcb.h>
#include <xcb/xcb_icccm.h>
#include <xcb/xcb_keysyms.h>
#include <X11/keysym.h>

#include "tree.h"

#define	BORDER_WIDTH			1
#define	BORDER_SCREEN_WIDTH		0
#define	BORDER_STATUS_WIDTH		1
#define	BORDER_WORKAREA_WIDTH		1
#define	BORDER_WORKSPACE_WIDTH		1
#define	BORDER_TILEFORK_WIDTH		0
#define	BORDER_TILE_WIDTH		1
#define	BORDER_TILE_ACTIVE_WIDTH       	1

#define	STATUS_HEIGHT	16

enum split {
	HSPLIT,
	VSPLIT,
};

enum window_type {
	WT_SCREEN,
	WT_STATUSBAR,
	WT_WORKAREA,
	WT_WORKSPACE,
	WT_TILEFORK,
	WT_TILE,
	WT_FRAME,
	WT_CLIENT,
};

struct wm {
	xcb_connection_t *conn;

	struct tree windows;

	struct tree screens_by_id;
	struct tree screens_by_window;

	struct tree tiles_by_id;
	struct tree tiles_by_window;

	struct tree curr_workarea;
	struct tree curr_status;
	struct tree curr_workspace;
	struct tree curr_tile;
	struct tree curr_frame;

	struct window *active_screen;
};

struct window {
	uint64_t		winid;
	uint64_t		objid;

	enum window_type        type;

    int	x;
    int	y;
    int	width;
    int	height;
	int	border_width;

	struct tree		children;

        xcb_screen_t           *xcb_screen;
        xcb_window_t            xcb_parent;
        xcb_window_t            xcb_window;
};


struct key {
	unsigned int		mod;
	xcb_keysym_t		ksym;
	void		       (*cb)(struct wm *, xcb_window_t);
};


/* event.c */
void		 event_loop(struct wm *wm);
void		 event_grab_keys(struct wm *wm, struct window *screen);


/* layout.c */
void		 layout_init(struct wm *wm);
void		 layout_screen_add(struct wm *wm, xcb_screen_t *xcb_screen);
void		 layout_finalize(struct wm *wm);
struct window	*layout_window_get(struct wm *wm, xcb_window_t xcb_window);
struct window	*layout_frame_get_current(struct wm *wm);
int		 layout_window_exists(struct wm *wm, xcb_window_t xcb_window);
struct window	*layout_client_create(struct wm *wm, xcb_window_t xcb_root, xcb_window_t xcb_window);
void		 layout_window_remove(struct wm *wm, xcb_window_t xcb_window);

void		 layout_workspace_create(struct wm *wm, xcb_window_t xcb_root);
void		 layout_workspace_next(struct wm *wm, xcb_window_t xcb_root);
void		 layout_workspace_prev(struct wm *wm, xcb_window_t xcb_root);
void		 layout_workspace_destroy(struct wm *wm, xcb_window_t xcb_root);

void		 layout_screen_register(struct wm *wm, xcb_screen_t *xcb_screen);
void		 layout_screen_render(struct wm *wm);

void		 layout_tile_prev(struct wm *wm, xcb_window_t xcb_root);
void		 layout_tile_next(struct wm *wm, xcb_window_t xcb_root);
void		 layout_tile_destroy(struct wm *wm, xcb_window_t xcb_root);

void		 layout_frame_prev(struct wm *wm, xcb_window_t xcb_root);
void		 layout_frame_next(struct wm *wm, xcb_window_t xcb_root);

void		 layout_tile_split(struct wm *wm, xcb_window_t xcb_root, enum split direction);
void		 layout_client_resize(struct wm *wm, struct window *client);
void		 layout_update(struct wm *wm);
void		 layout_update_status(struct wm *wm, struct window *status);
void		 layout_tile_set_active(struct wm *wm, xcb_window_t window);
void		 layout_client_destroy(struct wm *wm, xcb_window_t xcb_window);
void		 layout_window_resize(struct wm *wm, xcb_window_t window);


/* window.c */
struct window	*window_create(struct wm *wm, enum window_type wt, struct window *parent);
struct window	*window_create_root(struct wm *wm, struct window *window);
struct window	*window_create_screen(struct wm *wm, struct window *window);
struct window	*window_create_status(struct wm *wm, struct window *window);
struct window	*window_create_workarea(struct wm *wm, struct window *window);
struct window	*window_create_workspace(struct wm *wm, struct window *window);
struct window	*window_create_tilefork(struct wm *wm, struct window *window);
struct window	*window_create_tile(struct wm *wm, struct window *window);
struct window	*window_create_frame(struct wm *wm, struct window *window);
struct window	*window_create_client(struct wm *wm, struct window *window);

void		 window_map(struct wm *wm, struct window *window);
void		 window_unmap(struct wm *wm, struct window *window);
void		 window_destroy(struct wm *wm, struct window *window);
void		 window_raise(struct wm *wm, struct window *window);
void		 window_reparent(struct wm *wm, struct window *parent, struct window *window);
void		 window_resize(struct wm *wm, struct window *window);
void		 window_border_color(struct wm *wm, struct window *window, const char *rgb);
void		 window_border_width(struct wm *wm, struct window *window, uint32_t width);

/* wm.c */
void		 wm_workspace_create(struct wm *wm, xcb_window_t xcb_root);
void		 wm_workspace_destroy(struct wm *wm, xcb_window_t xcb_root);
void		 wm_workspace_next(struct wm *wm, xcb_window_t xcb_root);
void		 wm_workspace_prev(struct wm *wm, xcb_window_t xcb_root);

void		 wm_run_terminal(struct wm *wm, xcb_window_t xcb_root);
void		 wm_run_xeyes(struct wm *wm, xcb_window_t xcb_root);

void		 wm_tile_split_h(struct wm *wm, xcb_window_t xcb_root);
void		 wm_tile_split_v(struct wm *wm, xcb_window_t xcb_root);

void		 wm_tile_destroy(struct wm *wm, xcb_window_t xcb_root);
void		 wm_tile_next(struct wm *wm, xcb_window_t xcb_root);
void		 wm_tile_prev(struct wm *wm, xcb_window_t xcb_root);
