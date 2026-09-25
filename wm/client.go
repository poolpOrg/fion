package wm

// Client is a top-level window managed by fion.
type Client struct {
	// frame holding the client as a tab
	frame *Frame

	// UnmapNotify events caused by fion itself, such as reparenting the
	// mapped window into another frame, that must not be taken for the
	// client withdrawing.
	ignoreUnmap int

	// whether fion last mapped or unmapped the window: only the active tab
	// of a frame is mapped
	mapped bool

	// asked to close with WM_DELETE_WINDOW: asked again, it is killed
	closeRequested bool

	// asking for attention, with the urgency hint or EWMH's, until it has
	// the focus
	urgent bool
}
