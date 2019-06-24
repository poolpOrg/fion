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

static uint64_t	window_id;

static uint32_t
rgb_pixel(const char *rgb)
{
        char buf[3] = { 0, 0, 0 };
        uint32_t r, g, b;

        r = strtol(memcpy(buf, rgb+1, 2), NULL, 16);
        g = strtol(memcpy(buf, rgb+3, 2), NULL, 16);
        b = strtol(memcpy(buf, rgb+5, 2), NULL, 16);

        return ((r << 16) + (g << 8) + b);
        
}

struct window *
window_create_screen(struct wm *wm, struct window *window)
{
        uint32_t        mask = XCB_CW_BACK_PIXEL;
        uint32_t        values[2] = {
		rgb_pixel("#335599"),
        };
        xcb_create_window(wm->conn,
            XCB_COPY_FROM_PARENT,
            window->xcb_window,
            window->xcb_screen->root,
            window->x,
            window->y,
            window->width,
            window->height,
            0,
            XCB_WINDOW_CLASS_INPUT_OUTPUT,
            window->xcb_screen->root_visual,
            mask, values);
	window->id = ++window_id;
        return window;
}

struct window *
window_create_status(struct wm *wm, struct window *window)
{
        uint32_t        mask = XCB_CW_BACK_PIXEL|XCB_CW_BORDER_PIXEL;
        uint32_t        values[2] = {
		rgb_pixel("#000000"),
		rgb_pixel("#0000ff"),
        };
        xcb_create_window(wm->conn,
            XCB_COPY_FROM_PARENT,
            window->xcb_window,
            window->xcb_parent,
            window->x,
            window->y,
            window->width,
            window->height,
            window->border_width,
            XCB_WINDOW_CLASS_INPUT_OUTPUT,
            window->xcb_screen->root_visual,
            mask, values);
	window->id = ++window_id;
        return window;
}

struct window *
window_create_workarea(struct wm *wm, struct window *window)
{
        uint32_t        mask = XCB_CW_BACK_PIXEL|XCB_CW_BORDER_PIXEL;
        uint32_t        values[2] = {
		rgb_pixel("#000000"),
		rgb_pixel("#0000ff"),
        };
        xcb_create_window(wm->conn,
            XCB_COPY_FROM_PARENT,
            window->xcb_window,
            window->xcb_parent,
            window->x,
            window->y,
            window->width,
            window->height,
	    window->border_width,
            XCB_WINDOW_CLASS_INPUT_OUTPUT,
            window->xcb_screen->root_visual,
            mask, values);
	window->id = ++window_id;
        return window;
}

struct window *
window_create_workspace(struct wm *wm, struct window *window)
{
        uint32_t        mask = XCB_CW_BACK_PIXEL|XCB_CW_BORDER_PIXEL;
        uint32_t        values[2] = {
		rgb_pixel("#000000"),
		arc4random(),
        };
        xcb_create_window(wm->conn,
            XCB_COPY_FROM_PARENT,
            window->xcb_window,
            window->xcb_parent,
            window->x,
            window->y,
            window->width,
            window->height,
	    window->border_width,
            XCB_WINDOW_CLASS_INPUT_OUTPUT,
            window->xcb_screen->root_visual,
            mask, values);
	window->id = ++window_id;
        return window;
}

struct window *
window_create_tile(struct wm *wm, struct window *window)
{
        uint32_t        mask = XCB_CW_BACK_PIXEL|XCB_CW_BORDER_PIXEL;
        uint32_t        values[2] = {
		rgb_pixel("#000000"),
		rgb_pixel("#FFFFFF"),
        };
        
        xcb_create_window(wm->conn,
            XCB_COPY_FROM_PARENT,
            window->xcb_window,
            window->xcb_parent,
            window->x,
            window->y,
            window->width,
            window->height,
	    window->border_width,
            XCB_WINDOW_CLASS_INPUT_OUTPUT,
            window->xcb_screen->root_visual,
            mask, values);
	window->id = ++window_id;
        return window;
}

struct window *
window_create_client(struct wm *wm, struct window *window)
{
        uint32_t        mask = XCB_CW_BACK_PIXEL|XCB_CW_BORDER_PIXEL;
        uint32_t        values[2] = {
		rgb_pixel("#000000"),
		rgb_pixel("#ffffff"),
        };

        xcb_create_window(wm->conn,
            XCB_COPY_FROM_PARENT,
            window->xcb_window,
            window->xcb_parent,
            window->x,
            window->y,
            window->width,
            window->height,
	    window->border_width,
            XCB_WINDOW_CLASS_INPUT_OUTPUT,
            window->xcb_screen->root_visual,
            mask, values);
	window->id = ++window_id;
        return window;
}

void
window_map(struct wm *wm, struct window *window)
{
	xcb_map_window(wm->conn, window->xcb_window);
}

void
window_unmap(struct wm *wm, struct window *window)
{
	xcb_unmap_window(wm->conn, window->xcb_window);
}

void
window_raise(struct wm *wm, struct window *window)
{
        uint32_t value = XCB_STACK_MODE_ABOVE;

        xcb_configure_window(wm->conn, window->xcb_window, XCB_CONFIG_WINDOW_STACK_MODE, &value);
}

void
window_reparent(struct wm *wm, struct window *parent, struct window *window)
{
        xcb_reparent_window(wm->conn, window->xcb_window, parent->xcb_window, 0, 0);
}

void
window_resize(struct wm *wm, struct window *window)
{
        uint32_t mask =
            XCB_CONFIG_WINDOW_Y |
            XCB_CONFIG_WINDOW_X |
            XCB_CONFIG_WINDOW_WIDTH |
            XCB_CONFIG_WINDOW_HEIGHT;
        uint32_t values[4] = {
                window->x,
                window->y,
                window->width,
                window->height,
        };

        xcb_configure_window(wm->conn, window->xcb_window, mask, values);
}

void
window_border_color(struct wm *wm, struct window *window, const char *rgb_color)
{
        uint32_t        mask = XCB_CW_BORDER_PIXEL;
        uint32_t        values[1] = {
		rgb_pixel(rgb_color),
        };

	xcb_change_window_attributes(wm->conn, window->xcb_window, mask, values);
}

void
window_border_width(struct wm *wm, struct window *window, uint32_t width)
{
	struct window *parent = tree_xget(&wm->windows, window->xcb_parent);
	uint16_t mask =
	    XCB_CONFIG_WINDOW_BORDER_WIDTH;
	uint32_t value[1] = {
		width,
	};
	window->border_width = value[0];
	xcb_configure_window(wm->conn, window->xcb_window, mask, &value);
}
