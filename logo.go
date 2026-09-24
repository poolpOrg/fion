// Package fion holds the project's files that the window manager embeds.
package fion

import _ "embed"

// Logo is the project's logo, white on black, as a JPEG.
//
//go:embed fion.jpg
var Logo []byte
