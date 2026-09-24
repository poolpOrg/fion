package wm

// Client is a top-level window managed by fion.
type Client struct {
	// frame holding the client as a tab
	frame *Frame

	// UnmapNotify events caused by fion itself, such as reparenting the
	// mapped window into another frame, that must not be taken for the
	// client withdrawing.
	ignoreUnmap int
}
