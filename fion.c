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

#include <sys/wait.h>

#include <err.h>
#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "fion.h"
#include "log.h"


static void	fion_init(struct wm *);
static void	fion_done(struct wm *);
static void	fion_setup(struct wm *);
static void	usage(void);

extern char *__progname;

static void
usage(void)
{
	err(1, "usage: %s [-d]", __progname);
}

int
main(int argc, char *argv[])
{
	struct wm wm;
	int dflag, ch;
	
	dflag = 0;
	while ((ch = getopt(argc, argv, "d")) != -1) {
		switch (ch) {
		case 'd':
			dflag = 1;
			break;
		default:
			usage();
		}
	}
	argc -= optind;
	argv += optind;

	if (dflag) {
		log_init(1, 0);
		log_setverbose(2);
	}

	log_info("started");

	fion_init(&wm);
	fion_setup(&wm);

#if 0
	if (pledge("stdio proc exec", NULL) == -1)
		err(1, "pledge");
#endif

	event_loop(&wm);

	fion_done(&wm);
	log_info("exiting");
	return 0;
}

static void
fion_init(struct wm *wm)
{
	wm->conn = xcb_connect(NULL, NULL);
	if (xcb_connection_has_error(wm->conn))
		err(1, "xcb_connect");
}

static void
fion_done(struct wm *wm)
{
	xcb_disconnect(wm->conn);
}

static void
fion_setup(struct wm *wm)
{
	xcb_screen_iterator_t iter;
	uint64_t screen_id = 0;
	uint32_t value =
	    XCB_EVENT_MASK_KEY_PRESS |
	    XCB_EVENT_MASK_STRUCTURE_NOTIFY |
	    XCB_EVENT_MASK_SUBSTRUCTURE_NOTIFY |
	    XCB_EVENT_MASK_SUBSTRUCTURE_REDIRECT;

	layout_init(wm);
	iter = xcb_setup_roots_iterator(xcb_get_setup(wm->conn));
	for (; iter.rem; screen_id++, xcb_screen_next(&iter)) {
		if (xcb_request_check(wm->conn,
			xcb_change_window_attributes_checked(wm->conn,
			    iter.data->root,
			    XCB_CW_EVENT_MASK, &value)))
			err(1, "fion_setup");
		layout_screen_register(wm, iter.data);
	}
	layout_screen_render(wm);
}
