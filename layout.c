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

#include <err.h>
#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "fion.h"
#include "log.h"

static struct window *find_window(struct wm *wm, xcb_window_t xcb_window);
static struct window *xfind_window(struct wm *wm, xcb_window_t xcb_window);
static struct window *find_ancestor(struct wm *wm, struct window *node, enum window_type type);
static struct window *find_screen(struct wm *wm, xcb_window_t xcb_root);
static struct window *find_workarea(struct wm *wm, struct window *screen);
static struct window *find_workspace(struct wm *wm, struct window *screen);
static struct window *find_workspace_next(struct wm *wm, struct window *workspace);
static struct window *find_workspace_prev(struct wm *wm, struct window *workspace);
static struct window *find_active_tile(struct wm *wm, xcb_screen_t *xcb_screen);
static struct window *find_tile_next(struct wm *wm, struct window *tile);
static struct window *find_tile_prev(struct wm *wm, struct window *tile);
static struct window *find_active_frame(struct wm *wm, struct window *tile);

static struct window *create_screen(struct wm *wm, xcb_screen_t *xcb_screen);
static struct window *create_status(struct wm *wm, struct window *screen);
static struct window *create_workarea(struct wm *wm, struct window *screen);
static struct window *create_workspace(struct wm *wm, struct window *screen);
static struct window *create_tile(struct wm *wm, struct window *parent);
static struct window *create_tile_fork(struct wm *wm, struct window *tile);
static struct window *create_frame(struct wm *wm, struct window *tile);
static struct window *create_client(struct wm *wm, struct window *parent, xcb_window_t xcb_window);

static void prepare_screen(struct wm *wm, struct window *screen);
static void prepare_workspace(struct wm *wm, struct window *workspace);
static void prepare_tile(struct wm *wm, struct window *tile);
static void prepare_tile_fork(struct wm *wm, struct window *tile, struct window *parent);

static struct window *tile_split(struct wm *wm, struct window *tile, enum split direction);
static void tile_resize(struct wm *wm, struct window *tile);

static void frame_set_active(struct wm *wm, struct window *frame);


/* layout initialization */
void
layout_init(struct wm *wm)
{
	tree_init(&wm->windows);

	tree_init(&wm->screens);
	tree_init(&wm->frames);

	tree_init(&wm->curr_workarea);
	tree_init(&wm->curr_workspace);
	tree_init(&wm->curr_tile);
	tree_init(&wm->curr_frame);
}

void
layout_screen_register(struct wm *wm, xcb_screen_t *xcb_screen)
{
	struct window *screen = create_screen(wm, xcb_screen);

	if (wm->active_screen == NULL)
		wm->active_screen = screen;
}

void
layout_screen_render(struct wm *wm)
{
	void *iter;
	struct window *node;

	iter = NULL;
	while (tree_iter(&wm->screens, &iter, NULL, (void **)&node)) {
		prepare_screen(wm, node);
		window_map(wm, node);
	}
	xcb_flush(wm->conn);
}


/* window lookup functions */
static struct window *
find_window(struct wm *wm, xcb_window_t xcb_window)
{
	return tree_get(&wm->windows, xcb_window);
}

static struct window *
xfind_window(struct wm *wm, xcb_window_t xcb_window)
{
	return tree_xget(&wm->windows, xcb_window);
}

static struct window *
find_ancestor(struct wm *wm, struct window *node, enum window_type type)
{
	struct window *parent;

	do {
		parent = find_window(wm, node->xcb_parent);
		if (parent->type == type)
			return parent;
		node = parent;
	} while (node->type != WT_SCREEN);

	return (NULL);
}

static struct window *
find_screen(struct wm *wm, xcb_window_t xcb_root)
{
	return tree_get(&wm->screens, xcb_root);
}

static struct window *
find_workarea(struct wm *wm, struct window *screen)
{
	return tree_xget(&wm->curr_workarea, (uint64_t)screen->xcb_screen);
}

static struct window *
find_workspace(struct wm *wm, struct window *screen)
{
	return tree_xget(&wm->curr_workspace, (uint64_t)screen->xcb_screen);
}

static struct window *
find_workspace_next(struct wm *wm, struct window *workspace)
{
	struct window *workarea = find_ancestor(wm, workspace, WT_WORKAREA);
	struct window *node;
	void *iter;

	iter = NULL;
	if (tree_iterfrom(&workarea->children, &iter, workspace->id + 1, NULL, (void **)&node))
		return node;
	while (tree_iter(&workarea->children, &iter, NULL, (void **)&node))
		return node;
	return workspace;
}

static struct window *
find_workspace_prev(struct wm *wm, struct window *workspace)
{
	struct window *workarea = find_ancestor(wm, workspace, WT_WORKAREA);
	struct window *node;
	struct window *prev;
	void *iter;

	prev = iter = NULL;
	while (tree_iter(&workarea->children, &iter, NULL, (void **)&node)) {
		if (node->id == workspace->id) {
			if (prev)
				return prev;
			break;
		}
		prev = node;
	}
	while (tree_iter(&workarea->children, &iter, NULL, (void **)&node))
		;
	return node;
}

static struct window *
find_active_tile(struct wm *wm, xcb_screen_t *xcb_screen)
{
	return tree_xget(&wm->curr_tile, (uint64_t)xcb_screen);
}

static struct window *
find_tile_next(struct wm *wm, struct window *tile)
{
	return tile;
}

static struct window *
find_tile_prev(struct wm *wm, struct window *tile)
{
	return tile;
}

static struct window *
find_active_frame(struct wm *wm, struct window *tile)
{
	return tree_xget(&wm->curr_frame, tile->xcb_window);
}


/* high-level window creation functions */
static struct window *
create_screen(struct wm *wm, xcb_screen_t *xcb_screen)
{
	struct window *window;

	if ((window = calloc(1, sizeof(*window))) == NULL)
		return (NULL);

	window->type = WT_SCREEN;
	window->xcb_screen = xcb_screen;

	window->xcb_window = xcb_screen->root;
	window->border_width = BORDER_SCREEN_WIDTH;
	window->width = xcb_screen->width_in_pixels;
	window->height = xcb_screen->height_in_pixels;

	tree_init(&window->children);
	tree_xset(&wm->windows, window->xcb_window, window);
	tree_xset(&wm->screens, window->xcb_window, window);

	return window_create_screen(wm, window);
}

static struct window *
create_status(struct wm *wm, struct window *parent)
{
	struct window *window;

	if ((window = calloc(1, sizeof(*window))) == NULL)
		return (NULL);

	window->type = WT_STATUSBAR;
	window->xcb_screen = parent->xcb_screen;
	window->xcb_parent = parent->xcb_window;
	window->xcb_window = xcb_generate_id(wm->conn);

	window->border_width = BORDER_STATUS_WIDTH;
	window->x = window->y = parent->border_width;
	window->width = parent->width - window->border_width * 2;
	window->height = STATUS_HEIGHT;

	tree_init(&window->children);
	tree_xset(&wm->windows, window->xcb_window, window);

	window = window_create_status(wm, window);
	tree_xset(&parent->children, window->id, window);
	return window;
}

static struct window *
create_workarea(struct wm *wm, struct window *parent)
{
	struct window *window;

	if ((window = calloc(1, sizeof(*window))) == NULL)
		return (NULL);

	window->type = WT_WORKAREA;
	window->xcb_screen = parent->xcb_screen;
	window->xcb_parent = parent->xcb_window;
	window->xcb_window = xcb_generate_id(wm->conn);

	window->border_width = BORDER_WORKAREA_WIDTH;
	window->x = parent->border_width;
	window->y = parent->border_width + STATUS_HEIGHT;
	window->width = parent->width - window->border_width * 2;
	window->height = parent->height - STATUS_HEIGHT - window->border_width * 2;

	tree_init(&window->children);

	tree_xset(&wm->windows, window->xcb_window, window);
	tree_set(&wm->curr_workarea, (uint64_t)parent->xcb_screen, window);

	window = window_create_workarea(wm, window);
	tree_xset(&parent->children, window->id, window);
	return window;
}

static struct window *
create_workspace(struct wm *wm, struct window *parent)
{
	struct window *window;

	if ((window = calloc(1, sizeof(*window))) == NULL)
		return (NULL);

	window->type = WT_WORKSPACE;
	window->xcb_screen = parent->xcb_screen;
	window->xcb_parent = parent->xcb_window;
	window->xcb_window = xcb_generate_id(wm->conn);

	window->border_width = BORDER_WORKSPACE_WIDTH;
	window->width = parent->width - window->border_width * 2;
	window->height = parent->height - window->border_width * 2;

	tree_init(&window->children);
	tree_xset(&wm->windows, window->xcb_window, window);

	tree_set(&wm->curr_workspace, (uint64_t)parent->xcb_screen, window);

	window = window_create_workspace(wm, window);
	tree_xset(&parent->children, window->id, window);
	return window;
}

static struct window *
create_tile_fork(struct wm *wm, struct window *tile)
{
	struct window *parent = xfind_window(wm, tile->xcb_parent);
	struct window *window;

	if ((window = calloc(1, sizeof(*window))) == NULL)
		return (NULL);

	window->type = WT_TILE_CONTAINER;
	window->xcb_screen = tile->xcb_screen;
	window->xcb_parent = tile->xcb_parent;
	window->xcb_window = xcb_generate_id(wm->conn);

	window->border_width = BORDER_TILEFORK_WIDTH;
	window->width = tile->width;
	window->height = tile->height;
	
	tree_init(&window->children);
	tree_xset(&wm->windows, window->xcb_window, window);

	window = window_create_tile(wm, window);
	tree_xset(&parent->children, window->id, window);
	return window;
}

static struct window *
create_tile(struct wm *wm, struct window *parent)
{
	struct window *window;

	if ((window = calloc(1, sizeof(*window))) == NULL)
		return (NULL);

	window->type = WT_TILE;
	window->xcb_screen = parent->xcb_screen;
	window->xcb_parent = parent->xcb_window;
	window->xcb_window = xcb_generate_id(wm->conn);

	window->border_width = BORDER_TILE_WIDTH;
	window->width = parent->width - window->border_width * 2;
	window->height = parent->height - window->border_width * 2;
	
	tree_init(&window->children);
	tree_xset(&wm->windows, window->xcb_window, window);
	tree_set(&wm->curr_tile, (uint64_t)parent->xcb_screen, window);

	window = window_create_tile(wm, window);
	tree_xset(&parent->children, window->id, window);
	return window;
}

static struct window *
create_frame(struct wm *wm, struct window *parent)
{
	struct window *window;

	if ((window = calloc(1, sizeof(*window))) == NULL)
		return (NULL);

	window->type = WT_FRAME;
	window->xcb_screen = parent->xcb_screen;
	window->xcb_parent = parent->xcb_window;
	window->xcb_window = xcb_generate_id(wm->conn);

	window->border_width = BORDER_FRAME_WIDTH;
	window->width = parent->width - window->border_width * 2;
	window->height = parent->height - window->border_width * 2;

	tree_init(&window->children);
	tree_xset(&wm->windows, window->xcb_window, window);

	tree_set(&wm->curr_frame, parent->xcb_window, window);

	window = window_create_frame(wm, window);
	tree_xset(&parent->children, window->id, window);
	return window;
}

static struct window *
create_client(struct wm *wm, struct window *parent, xcb_window_t xcb_window)
{
	struct window *window;

	if ((window = calloc(1, sizeof(*window))) == NULL)
		return (NULL);

	window->type = WT_CLIENT;
	window->xcb_screen = parent->xcb_screen;
	window->xcb_parent = parent->xcb_window;
	window->xcb_window = xcb_window;

	window->width = parent->width - window->border_width * 2;
	window->height = parent->height - window->border_width * 2;

	tree_init(&window->children);
	tree_set(&wm->windows, window->xcb_window, window);

	window = window_create_client(wm, window);
	tree_xset(&parent->children, window->id, window);
	return window;
}


/* prepare window-specific setup */
static void
prepare_screen(struct wm *wm, struct window *screen)
{
	struct window *window;

	window = create_status(wm, screen);
	window_map(wm, window);

	window = create_workarea(wm, screen);
	window_map(wm, window);

	window = create_workspace(wm, window);
	window_map(wm, window);

	prepare_workspace(wm, window);
}

static void
prepare_workspace(struct wm *wm, struct window *workspace)
{
	struct window *tile;
	struct window *parent;

	tile = create_tile(wm, workspace);
	parent = create_tile_fork(wm, tile);

	prepare_tile_fork(wm, tile, parent);
	prepare_tile(wm, tile);

	window_map(wm, tile);
	window_map(wm, parent);
}

static void
prepare_tile(struct wm *wm, struct window *tile)
{
	struct window *frame;

	frame = create_frame(wm, tile);
	window_map(wm, frame);

	if (wm->active_frame == NULL)
		frame_set_active(wm, frame);
}

static void
prepare_tile_fork(struct wm *wm, struct window *tile, struct window *parent)
{
	tile->width = parent->width - tile->border_width * 2;
	tile->height = parent->height - tile->border_width * 2;
	window_resize(wm, tile);

	tile->xcb_parent = parent->xcb_window;
	window_reparent(wm, parent, tile);
}


/* tile */
static void
tile_set_active(struct wm *wm, struct window *tile)
{
	tree_set(&wm->curr_tile, (uint64_t)tile->xcb_screen, tile);
}

static struct window *
tile_split(struct wm *wm, struct window *tile, enum split direction)
{
	struct window *parent;
	struct window *sibling;

	/* 1- create a clone tile that will become parent */
	parent = create_tile_fork(wm, tile);

	/* 2- create a sibling tile */
	sibling = create_tile(wm, parent);
	prepare_tile(wm, sibling);

	/* 3- reparent tile to new parent */
	prepare_tile_fork(wm, tile, parent);

	/* 4- recompute tile and sibling sizes */
	switch (direction) {
	case HSPLIT:
		tile->width = parent->width - 2 * tile->border_width;
		sibling->width = tile->width;

		tile->height = (parent->height / 2) - 2 * tile->border_width;
		sibling->height = tile->height;

		tile->y = 0;
		sibling->y = tile->height + tile->border_width * 2;
		if (parent->height & 1)
			sibling->height += 1;
		break;

	case VSPLIT:
		tile->height = parent->height - 2 * tile->border_width;
		sibling->height = tile->height;

		tile->width = (parent->width / 2) - 2 * tile->border_width;
		sibling->width = tile->width;

		tile->x = 0;
		sibling->x = tile->width + tile->border_width * 2;
		if (parent->width & 1) 
			sibling->width += 1;
                break;
	}
	return sibling;
}

static void
tile_resize(struct wm *wm, struct window *tile)
{
	void *iter;
	struct window *node;
	
	iter = NULL;
	while (tree_iter(&tile->children, &iter, NULL, (void **)&node)) {
		node->x = node->y = 0;
		node->height = tile->height - node->border_width * 2;
		node->width = tile->width - node->border_width * 2;
		window_resize(wm, node);
		if (node->type == WT_TILE_CONTAINER || node->type == WT_TILE || node->type == WT_FRAME)
			tile_resize(wm, node);
	}
}


/* frame */
static void
frame_set_active(struct wm *wm, struct window *frame)
{
	if (wm->active_frame) {
		window_border_width(wm, wm->active_frame, 1);
		window_border_color(wm, wm->active_frame, "#ffffff");
	}
	wm->active_frame = frame;
	window_border_width(wm, wm->active_frame, 3);
	window_border_color(wm, wm->active_frame, "#ff0000");
}


/* client */
struct window *
layout_client_create(struct wm *wm, xcb_window_t xcb_root, xcb_window_t xcb_window)
{
	struct window *window = xfind_window(wm, xcb_root);
	struct window *tile = find_active_tile(wm, window->xcb_screen);
	struct window *frame = find_active_frame(wm, tile);
	struct window *client;

	client = create_client(wm, frame, xcb_window);
	
	if (find_window(wm, client->xcb_window) == NULL)
		tree_xset(&wm->windows, client->xcb_window, client);

	window_reparent(wm, frame, client);
	window_resize(wm, client);

	return (client);
}


/* window management */
struct window *
layout_window_get(struct wm *wm, xcb_window_t xcb_window)
{
	return tree_get(&wm->windows, xcb_window);
}

int
layout_window_exists(struct wm *wm, xcb_window_t xcb_window)
{
	return tree_get(&wm->windows, xcb_window) ? 1 : 0;
}

void
layout_window_remove(struct wm *wm, xcb_window_t xcb_window)
{
	struct window	*window = tree_xpop(&wm->windows, xcb_window);

	switch (window->type) {
	case WT_CLIENT:
	case WT_SCREEN:
	case WT_STATUSBAR:
	case WT_WORKAREA:
	case WT_WORKSPACE:
	case WT_TILE_CONTAINER:
	case WT_TILE:
	case WT_FRAME:
		err(1, "can't remove this window");
	}

	free(window);
}

/* user commands */
void
layout_tile_split(struct wm *wm, xcb_window_t xcb_root, enum split direction)
{
	struct window *window = xfind_window(wm, xcb_root);
	struct window *tile = find_active_tile(wm, window->xcb_screen);
	struct window *sibling;

	window_unmap(wm, tile);

	sibling = tile_split(wm, tile, direction);

	tile_set_active(wm, tile);
	
	window_resize(wm, sibling);
	window_resize(wm, tile);

	tile_resize(wm, sibling);
	tile_resize(wm, tile);

	window_map(wm, find_ancestor(wm, sibling, WT_TILE_CONTAINER));
	window_map(wm, sibling);
	window_map(wm, tile);
}

void
layout_tile_next(struct wm *wm, xcb_window_t xcb_root)
{
	struct window *window = xfind_window(wm, xcb_root);
	struct window *tile = find_active_tile(wm, window->xcb_screen);
	struct window *next = find_tile_next(wm, tile);

	if (next == tile)
		return;
}

void
layout_tile_prev(struct wm *wm, xcb_window_t xcb_root)
{
	struct window *window = xfind_window(wm, xcb_root);
	struct window *tile = find_active_tile(wm, window->xcb_screen);
	struct window *prev = find_tile_prev(wm, tile);
	
	if (prev == tile)
		return;
}


void
layout_workspace_create(struct wm *wm, xcb_window_t xcb_root)
{
	struct window *screen = find_screen(wm, xcb_root);
	struct window *workarea = find_workarea(wm, screen);
	struct window *workspace = find_workspace(wm, screen);
	struct window *window;

	window = create_workspace(wm, workarea);
	prepare_workspace(wm, window);
	window_map(wm, window);
	window_unmap(wm, workspace);
}

void
layout_workspace_destroy(struct wm *wm, xcb_window_t xcb_root)
{
	struct window *screen = find_screen(wm, xcb_root);
	struct window *workarea = find_workarea(wm, screen);
	struct window *workspace = find_workspace(wm, screen);
	struct window *next;

	tree_xpop(&workarea->children, workspace->id);
	if (! tree_root(&workarea->children, NULL, (void **)&next)) {
		/* removing last workspace is not allowed */
		tree_xset(&workarea->children, workspace->id, workspace);
		return;
	}

	window_map(wm, next);
	tree_set(&wm->curr_workspace, (uint64_t)screen->xcb_screen, next);	
	window_unmap(wm, workspace);	
}

void
layout_workspace_next(struct wm *wm, xcb_window_t xcb_root)
{
	struct window *screen = find_screen(wm, xcb_root);
	struct window *workspace = find_workspace(wm, screen);
	struct window *next = find_workspace_next(wm, workspace);

	if (next == workspace)
		return;

	window_map(wm, next);
	tree_set(&wm->curr_workspace, (uint64_t)screen->xcb_screen, next);
	window_unmap(wm, workspace);
}

void
layout_workspace_prev(struct wm *wm, xcb_window_t xcb_root)
{
	struct window *screen = find_screen(wm, xcb_root);
	struct window *workspace = find_workspace(wm, screen);
	struct window *prev = find_workspace_prev(wm, workspace);

	if (prev == workspace)
		return;

	window_map(wm, prev);
	tree_set(&wm->curr_workspace, (uint64_t)screen->xcb_screen, prev);
	window_unmap(wm, workspace);
}
