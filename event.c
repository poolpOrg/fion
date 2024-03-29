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
#include <poll.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "fion.h"
#include "log.h"


#define	KBMODE_WORKSPACE	1
#define	KBMODE_TILE		2
#define	KBMODE_RUN		3


static int		running = 1;
static int		mode;
static inline void	event_quit(struct wm *wm, xcb_window_t screen) { running = 0; }
static inline void	event_workspace(struct wm *wm, xcb_window_t screen) { mode = KBMODE_WORKSPACE; log_debug("workspace mode"); }
static inline void	event_tile(struct wm *wm, xcb_window_t screen) { mode = KBMODE_TILE; log_debug("tile mode"); }
static inline void	event_run(struct wm *wm, xcb_window_t screen) { mode = KBMODE_RUN; log_debug("run mode"); }

static inline void
kb_c(struct wm *wm, xcb_window_t screen)
{
	switch (mode) {
	case KBMODE_WORKSPACE:
		wm_workspace_create(wm, screen);
		break;
	}
	mode = 0;
}

static inline void
kb_d(struct wm *wm, xcb_window_t screen)
{
	switch (mode) {
	case KBMODE_WORKSPACE:
		wm_workspace_destroy(wm, screen);
		break;
	case KBMODE_TILE:
		wm_tile_destroy(wm, screen);
		break;
	}
	mode = 0;
}

static inline void
kb_n(struct wm *wm, xcb_window_t screen)
{
	switch (mode) {
	case KBMODE_WORKSPACE:
		wm_workspace_next(wm, screen);
		break;
	case KBMODE_TILE:
		wm_tile_next(wm, screen);
		break;
	}
	mode = 0;
}

static inline void
kb_p(struct wm *wm, xcb_window_t screen)
{
	switch (mode) {
	case KBMODE_WORKSPACE:
		wm_workspace_prev(wm, screen);
		break;
	case KBMODE_TILE:
		wm_tile_prev(wm, screen);
		break;
	}
	mode = 0;
}

static inline void
kb_h(struct wm *wm, xcb_window_t screen)
{
	switch (mode) {
	case KBMODE_TILE:
		wm_tile_split_h(wm, screen);
		break;
	}
	mode = 0;
}

static inline void
kb_v(struct wm *wm, xcb_window_t screen)
{
	switch (mode) {
	case KBMODE_TILE:
		wm_tile_split_v(wm, screen);
		break;
	}
	mode = 0;
}

static inline void
kb_t(struct wm *wm, xcb_window_t screen)
{
	switch (mode) {
	case KBMODE_RUN:
		wm_run_terminal(wm, screen);
		break;
	}
	mode = 0;
}

static struct key	keys[] = {
	{ XCB_MOD_MASK_4,	XK_q,		event_quit },

	{ XCB_MOD_MASK_4,	XK_w,		event_workspace },
	{ XCB_MOD_MASK_4,	XK_t,		event_tile },
	{ XCB_MOD_MASK_4,	XK_r,		event_run },

	{ 0,	XK_c,		kb_c },		/* create */
	{ 0,	XK_d,		kb_d },		/* destroy */
	{ 0,	XK_n,		kb_n },		/* next */
	{ 0,	XK_p,		kb_p },		/* prev */

	{ 0,	XK_h,		kb_h },		/* horizontal */
	{ 0,	XK_v,		kb_v },		/* vertical */

	{ 0,	XK_t,		kb_t },		/* terminal */
};

static void	on_key_press(struct wm *wm, xcb_key_press_event_t *ev);
static void	on_key_release(struct wm *wm, xcb_key_release_event_t *ev);
static void	on_button_press(struct wm *wm, xcb_button_press_event_t *ev);
static void	on_button_release(struct wm *wm, xcb_button_release_event_t *ev);
static void	on_motion_notify(struct wm *wm, xcb_motion_notify_event_t *ev);
static void	on_enter_notify(struct wm *wm, xcb_enter_notify_event_t *ev);
static void	on_leave_notify(struct wm *wm, xcb_leave_notify_event_t *ev);
static void	on_focus_in(struct wm *wm, xcb_focus_in_event_t *ev);
static void	on_focus_out(struct wm *wm, xcb_focus_out_event_t *ev);
static void	on_keymap_notify(struct wm *wm, xcb_keymap_notify_event_t *ev);
static void	on_expose(struct wm *wm, xcb_expose_event_t *ev);
static void	on_graphics_exposure(struct wm *wm, xcb_graphics_exposure_event_t *ev);
static void	on_no_exposure(struct wm *wm, xcb_no_exposure_event_t *ev);
static void	on_visibility_notify(struct wm *wm, xcb_visibility_notify_event_t *ev);
static void	on_create_notify(struct wm *wm, xcb_create_notify_event_t *ev);
static void	on_destroy_notify(struct wm *wm, xcb_destroy_notify_event_t *ev);
static void	on_unmap_notify(struct wm *wm, xcb_unmap_notify_event_t *ev);
static void	on_map_notify(struct wm *wm, xcb_map_notify_event_t *ev);
static void	on_map_request(struct wm *wm, xcb_map_request_event_t *ev);
static void	on_reparent_notify(struct wm *wm, xcb_reparent_notify_event_t *ev);
static void	on_configure_notify(struct wm *wm, xcb_configure_notify_event_t *ev);
static void	on_configure_request(struct wm *wm, xcb_configure_request_event_t *ev);
static void	on_gravity_notify(struct wm *wm, xcb_gravity_notify_event_t *ev);
static void	on_resize_request(struct wm *wm, xcb_resize_request_event_t *ev);
static void	on_circulate_notify(struct wm *wm, xcb_circulate_notify_event_t *ev);
static void	on_circulate_request(struct wm *wm, xcb_circulate_request_event_t *ev);
static void	on_property_notify(struct wm *wm, xcb_property_notify_event_t *ev);
static void	on_selection_clear(struct wm *wm, xcb_selection_clear_event_t *ev);
static void	on_selection_request(struct wm *wm, xcb_selection_request_event_t *ev);
static void	on_selection_notify(struct wm *wm, xcb_selection_notify_event_t *ev);
static void	on_colormap_notify(struct wm *wm, xcb_colormap_notify_event_t *ev);
static void	on_client_message(struct wm *wm, xcb_client_message_event_t *ev);
static void	on_mapping_notify(struct wm *wm, xcb_mapping_notify_event_t *ev);
static void	on_ge_generic(struct wm *wm, xcb_ge_generic_event_t *ev);

void
event_grab_keys(struct wm *wm, struct window *screen)
{
	int i;
	int j;
	xcb_key_symbols_t *ksyms;
	xcb_keycode_t *kcode;

	ksyms = xcb_key_symbols_alloc(wm->conn);
	for (i = 0; (unsigned long)i < sizeof(keys) / sizeof(struct key); ++i) {
		kcode = xcb_key_symbols_get_keycode(ksyms, keys[i].ksym);
		for (j = 0; kcode[j] != XCB_NO_SYMBOL; ++j)
			xcb_grab_key(wm->conn, 1, screen->xcb_window,
			    keys[j].mod, kcode[j],
			    XCB_GRAB_MODE_ASYNC, XCB_GRAB_MODE_ASYNC);
		free(kcode);
	}
	xcb_key_symbols_free(ksyms);
}

static void
event_process(struct wm *wm, xcb_generic_event_t *e)
{
	switch (e->response_type & ~0x80) {
	case XCB_KEY_PRESS:
		on_key_press(wm, (xcb_key_press_event_t *)e);
		break;

	case XCB_KEY_RELEASE:
		on_key_release(wm, (xcb_key_release_event_t *)e);
		break;

	case XCB_BUTTON_PRESS:
		on_button_press(wm, (xcb_button_press_event_t *)e);
		break;

	case XCB_BUTTON_RELEASE:
		on_button_release(wm, (xcb_button_release_event_t *)e);
		break;

	case XCB_MOTION_NOTIFY:
		on_motion_notify(wm, (xcb_motion_notify_event_t *)e);
		break;

	case XCB_ENTER_NOTIFY:
		on_enter_notify(wm, (xcb_enter_notify_event_t *)e);
		break;

	case XCB_LEAVE_NOTIFY:
		on_leave_notify(wm, (xcb_leave_notify_event_t *)e);
		break;

	case XCB_FOCUS_IN:
		on_focus_in(wm, (xcb_focus_in_event_t *)e);
		break;

	case XCB_FOCUS_OUT:
		on_focus_out(wm, (xcb_focus_out_event_t *)e);
		break;

	case XCB_KEYMAP_NOTIFY:
		on_keymap_notify(wm, (xcb_keymap_notify_event_t *)e);
		break;

	case XCB_EXPOSE:
		on_expose(wm, (xcb_expose_event_t *)e);
		break;

	case XCB_GRAPHICS_EXPOSURE:
		on_graphics_exposure(wm, (xcb_graphics_exposure_event_t *)e);
		break;

	case XCB_NO_EXPOSURE:
		on_no_exposure(wm, (xcb_no_exposure_event_t *)e);
		break;

	case XCB_VISIBILITY_NOTIFY:
		on_visibility_notify(wm, (xcb_visibility_notify_event_t *)e);
		break;

	case XCB_CREATE_NOTIFY:
		on_create_notify(wm, (xcb_create_notify_event_t *)e);
		break;

	case XCB_DESTROY_NOTIFY:
		on_destroy_notify(wm, (xcb_destroy_notify_event_t *)e);
		break;

	case XCB_UNMAP_NOTIFY:
		on_unmap_notify(wm, (xcb_unmap_notify_event_t *)e);
		break;

	case XCB_MAP_NOTIFY:
		on_map_notify(wm, (xcb_map_notify_event_t *)e);
		break;

	case XCB_MAP_REQUEST:
		on_map_request(wm, (xcb_map_request_event_t *)e);
		break;

	case XCB_REPARENT_NOTIFY:
		on_reparent_notify(wm, (xcb_reparent_notify_event_t *)e);
		break;

	case XCB_CONFIGURE_NOTIFY:
		on_configure_notify(wm, (xcb_configure_notify_event_t *)e);
		break;

	case XCB_CONFIGURE_REQUEST:
		on_configure_request(wm, (xcb_configure_request_event_t *)e);
		break;

	case XCB_GRAVITY_NOTIFY:
		on_gravity_notify(wm, (xcb_gravity_notify_event_t *)e);
		break;

	case XCB_RESIZE_REQUEST:
		on_resize_request(wm, (xcb_resize_request_event_t *)e);
		break;

	case XCB_CIRCULATE_NOTIFY:
		on_circulate_notify(wm, (xcb_circulate_notify_event_t *)e);
		break;

	case XCB_CIRCULATE_REQUEST:
		on_circulate_request(wm, (xcb_circulate_request_event_t *)e);
		break;

	case XCB_PROPERTY_NOTIFY:
		on_property_notify(wm, (xcb_property_notify_event_t *)e);
		break;

	case XCB_SELECTION_CLEAR:
		on_selection_clear(wm, (xcb_selection_clear_event_t *)e);
		break;

	case XCB_SELECTION_REQUEST:
		on_selection_request(wm, (xcb_selection_request_event_t *)e);
		break;

	case XCB_SELECTION_NOTIFY:
		on_selection_notify(wm, (xcb_selection_notify_event_t *)e);
		break;

	case XCB_COLORMAP_NOTIFY:
		on_colormap_notify(wm, (xcb_colormap_notify_event_t *)e);
		break;

	case XCB_CLIENT_MESSAGE:
		on_client_message(wm, (xcb_client_message_event_t *)e);
		break;

	case XCB_MAPPING_NOTIFY:
		on_mapping_notify(wm, (xcb_mapping_notify_event_t *)e);
		break;

	case XCB_GE_GENERIC:
		on_ge_generic(wm, (xcb_ge_generic_event_t *)e);
		break;

	case 0:
	case 1:
		/* xproto.h documents opcodes starting at 2 */
		break;

	default:
		log_warnx("received unknown event \"%d\"", e->response_type & ~0x80);
	}
	xcb_flush(wm->conn);
}

void
event_loop(struct wm *wm)
{
	xcb_generic_event_t *e;
	struct pollfd pfd[1];
	int nready;

	pfd[0].fd = xcb_get_file_descriptor(wm->conn);
	pfd[0].events = POLLIN;
	do {
		/* tick every 0.1s */
		nready = poll(pfd, 1, 1 * 100);
		if (nready == -1)
			err(1, "poll");

		if (nready) {
			while ((e = xcb_poll_for_event(wm->conn)) != NULL) {
				event_process(wm, e);
				free(e);
			}
		}
		layout_update(wm);
		xcb_flush(wm->conn);
	} while (running);
}


static void
on_key_press(struct wm *wm, xcb_key_press_event_t *ev)
{
	xcb_key_symbols_t      *ksyms;
	xcb_keysym_t		ksym;
	int			i;

	ksyms = xcb_key_symbols_alloc(wm->conn);
	ksym = xcb_key_symbols_get_keysym(ksyms, ev->detail, 0);
	xcb_key_symbols_free(ksyms);

	for (i = 0; (size_t)i < sizeof(keys) / sizeof(struct key); ++i)
		if ((keys[i].mod == 0 || (ev->state & keys[i].mod)) && ksym == keys[i].ksym && keys[i].cb) {
			keys[i].cb(wm, ev->root);
			return;
		}
	mode = 0;
}

static void
on_key_release(struct wm *wm, xcb_key_release_event_t *ev)
{
	log_debug("on_key_release");
}

static void
on_button_press(struct wm *wm, xcb_button_press_event_t *ev)
{
	log_debug("on_button_press");
}

static void
on_button_release(struct wm *wm, xcb_button_release_event_t *ev)
{
	log_debug("on_button_release");
}

static void
on_motion_notify(struct wm *wm, xcb_motion_notify_event_t *ev)
{
	log_debug("on_motion_notify");
}

static void
on_enter_notify(struct wm *wm, xcb_enter_notify_event_t *ev)
{
	/*log_debug("on_enter_notify");*/
	layout_tile_set_active(wm, ev->event);
}

static void
on_leave_notify(struct wm *wm, xcb_leave_notify_event_t *ev)
{
	/*log_debug("on_leave_notify");*/
}

static void
on_focus_in(struct wm *wm, xcb_focus_in_event_t *ev)
{
	log_debug("on_focus_in");
}

static void
on_focus_out(struct wm *wm, xcb_focus_out_event_t *ev)
{
	log_debug("on_focus_out");
}

static void
on_keymap_notify(struct wm *wm, xcb_keymap_notify_event_t *ev)
{
	log_debug("on_keymap_notify");
}

static void
on_expose(struct wm *wm, xcb_expose_event_t *ev)
{
	log_debug("on_expose");
}

static void
on_graphics_exposure(struct wm *wm, xcb_graphics_exposure_event_t *ev)
{
	log_debug("on_graphics_exposure");
}

static void
on_no_exposure(struct wm *wm, xcb_no_exposure_event_t *ev)
{
	log_debug("on_no_exposure");
}

static void
on_visibility_notify(struct wm *wm, xcb_visibility_notify_event_t *ev)
{
	log_debug("on_visibility_notify");
}

static void
on_create_notify(struct wm *wm, xcb_create_notify_event_t *ev)
{
	struct window *window = layout_window_get(wm, ev->window);

	log_debug("on_create_notify: %lld", (long long)ev->window);

	if (window == NULL) {
		log_debug("creating new client window");
		layout_client_create(wm, ev->parent, ev->window);
	}
	else {
		log_debug("reusing window... woops: %d", window->type);
	}
}

static void
on_destroy_notify(struct wm *wm, xcb_destroy_notify_event_t *ev)
{
	log_debug("on_destroy_notify: %lld", (long long)ev->window);
	layout_client_destroy(wm, ev->window);
}

static void
on_unmap_notify(struct wm *wm, xcb_unmap_notify_event_t *ev)
{
	/*log_debug("on_unmap_notify");*/
}

static void
on_map_notify(struct wm *wm, xcb_map_notify_event_t *ev)
{
	/*log_debug("on_map_notify");*/
}

static void
on_map_request(struct wm *wm, xcb_map_request_event_t *ev)
{
	/*log_debug("on_map_request");*/
	xcb_map_window(wm->conn, ev->window);
}

static void
on_reparent_notify(struct wm *wm, xcb_reparent_notify_event_t *ev)
{
	log_debug("on_reparent_notify: %d", ev->window);
}

static void
on_configure_notify(struct wm *wm, xcb_configure_notify_event_t *ev)
{
	log_debug("on_configure_notify: %d", ev->window);
	layout_window_resize(wm, ev->window);
}

static void
on_configure_request(struct wm *wm, xcb_configure_request_event_t *ev)
{
	log_debug("on_configure_request");
}

static void
on_gravity_notify(struct wm *wm, xcb_gravity_notify_event_t *ev)
{
	log_debug("on_gravity_notify");
}

static void
on_resize_request(struct wm *wm, xcb_resize_request_event_t *ev)
{
	log_debug("on_resize_request");
}

static void
on_circulate_notify(struct wm *wm, xcb_circulate_notify_event_t *ev)
{
	log_debug("on_circulate_notify");
}

static void
on_circulate_request(struct wm *wm, xcb_circulate_request_event_t *ev)
{
	log_debug("on_circulate_request");
}

static void
on_property_notify(struct wm *wm, xcb_property_notify_event_t *ev)
{
	log_debug("on_property_notify");
}

static void
on_selection_clear(struct wm *wm, xcb_selection_clear_event_t *ev)
{
	log_debug("on_selection_clear");
}

static void
on_selection_request(struct wm *wm, xcb_selection_request_event_t *ev)
{
	log_debug("on_selection_request");
}

static void
on_selection_notify(struct wm *wm, xcb_selection_notify_event_t *ev)
{
	log_debug("on_selection_notify");
}

static void
on_colormap_notify(struct wm *wm, xcb_colormap_notify_event_t *ev)
{
	log_debug("on_colormap_notify");
}

static void
on_client_message(struct wm *wm, xcb_client_message_event_t *ev)
{
	log_debug("on_client_message");
}

static void
on_mapping_notify(struct wm *wm, xcb_mapping_notify_event_t *ev)
{
	/*log_debug("on_mapping_notify");*/
}

static void	on_ge_generic(struct wm *wm, xcb_ge_generic_event_t *ev)
{
	log_debug("on_ge_generic");
}
