package main

import "github.com/BurntSushi/xgb/xproto"

type Client struct {
	Win   xproto.Window
	Float bool // floating client bypasses frame tree
}
