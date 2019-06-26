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
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "fion.h"
#include "log.h"

void
wm_run_terminal(struct wm *wm, xcb_window_t xcb_root)
{
	log_debug("run_terminal");
	pid_t	pid;

	if ((pid = fork()) < 0) {
		warn("wm_run_terminal");
		return;
	}
	if (pid)
		return;

	execlp("xterm", "xterm", "-fg", "white", "-bg", "black", NULL);
	exit(1);
}

void
wm_run_xeyes(struct wm *wm, xcb_window_t xcb_root)
{
	log_debug("run_xeyes");
	pid_t	pid;

	if ((pid = fork()) < 0) {
		warn("wm_run_xeyes");
		return;
	}
	if (pid)
		return;

	execlp("xeyes", "xeyes", NULL);
	exit(1);
}


void
wm_workspace_create(struct wm *wm, xcb_window_t xcb_root)
{
	log_debug("workspace_create");
	layout_workspace_create(wm, xcb_root);
}

void
wm_workspace_destroy(struct wm *wm, xcb_window_t xcb_root)
{
	log_debug("workspace_destroy");
	layout_workspace_destroy(wm, xcb_root);
}

void
wm_workspace_next(struct wm *wm, xcb_window_t xcb_root)
{
	log_debug("workspace_next");
	layout_workspace_next(wm, xcb_root);
}

void
wm_workspace_prev(struct wm *wm, xcb_window_t xcb_root)
{
	log_debug("workspace_prev");
	layout_workspace_prev(wm, xcb_root);
}

void
wm_tile_split_h(struct wm *wm, xcb_window_t xcb_root)
{
	log_debug("tile_split_h");
	layout_tile_split(wm, xcb_root, HSPLIT);
}

void
wm_tile_split_v(struct wm *wm, xcb_window_t xcb_root)
{
	log_debug("tile_split_v");
	layout_tile_split(wm, xcb_root, VSPLIT);
}

void
wm_tile_next(struct wm *wm, xcb_window_t xcb_root)
{
	log_debug("tile_next");
	layout_tile_next(wm, xcb_root);
}

void
wm_tile_prev(struct wm *wm, xcb_window_t xcb_root)
{
	log_debug("tile_prev");
	layout_tile_prev(wm, xcb_root);
}
