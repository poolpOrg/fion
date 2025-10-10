package wm

import (
	"log"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

type Atoms struct {
	WM_PROTOCOLS, WM_DELETE_WINDOW, WM_TAKE_FOCUS, WM_STATE                    xproto.Atom
	NET_SUPPORTING_WM_CHECK, NET_SUPPORTED, NET_CLIENT_LIST, NET_ACTIVE_WINDOW xproto.Atom
	NET_WM_NAME, UTF8_STRING, NET_WM_WINDOW_TYPE, NET_WM_WINDOW_TYPE_DOCK      xproto.Atom
}

func internAtom(X *xgb.Conn, name string) xproto.Atom {
	rep, err := xproto.InternAtom(X, false, uint16(len(name)), name).Reply()
	if err != nil {
		log.Fatalf("InternAtom %s: %v", name, err)
	}
	return rep.Atom
}

func getAtoms(X *xgb.Conn) Atoms {
	return Atoms{
		WM_PROTOCOLS:            internAtom(X, "WM_PROTOCOLS"),
		WM_DELETE_WINDOW:        internAtom(X, "WM_DELETE_WINDOW"),
		WM_TAKE_FOCUS:           internAtom(X, "WM_TAKE_FOCUS"),
		WM_STATE:                internAtom(X, "WM_STATE"),
		NET_SUPPORTING_WM_CHECK: internAtom(X, "_NET_SUPPORTING_WM_CHECK"),
		NET_SUPPORTED:           internAtom(X, "_NET_SUPPORTED"),
		NET_CLIENT_LIST:         internAtom(X, "_NET_CLIENT_LIST"),
		NET_ACTIVE_WINDOW:       internAtom(X, "_NET_ACTIVE_WINDOW"),
		NET_WM_NAME:             internAtom(X, "_NET_WM_NAME"),
		UTF8_STRING:             internAtom(X, "UTF8_STRING"),
		NET_WM_WINDOW_TYPE:      internAtom(X, "_NET_WM_WINDOW_TYPE"),
		NET_WM_WINDOW_TYPE_DOCK: internAtom(X, "_NET_WM_WINDOW_TYPE_DOCK"),
	}
}
