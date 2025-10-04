package main

import "github.com/BurntSushi/xgb/xproto"

type Client struct {
	Win   xproto.Window
	Frame xproto.Window
	Float bool // floating client bypasses frame tree
}

func (wm *WM) clientByFrame(frame xproto.Window) *Client {
	for _, cl := range wm.Clients {
		if cl.Frame == frame {
			return cl
		}
	}
	return nil
}

func (wm *WM) clientByLeaf(leaf *FrameNode) *Client {
	if leaf.Kind != Leaf || leaf.Active < 0 || leaf.Active >= len(leaf.Tabs) {
		return nil
	}
	w := leaf.Tabs[leaf.Active]
	return wm.Clients[w]
}
