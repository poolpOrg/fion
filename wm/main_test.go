package wm

import (
	"os"
	"testing"

	"github.com/jezek/xgb"
)

// TestMain readies RandR before the tests open their connections: its
// first initialization writes a table of xgb's that the connections'
// readers share.
func TestMain(m *testing.M) {
	var conn *xgb.Conn
	if display := os.Getenv("FION_TEST_DISPLAY"); display != "" {
		if c, err := xgb.NewConnDisplay(display); err == nil {
			initRandr(c)
			conn = c
		}
	}
	code := m.Run()
	if conn != nil {
		conn.Close()
	}
	os.Exit(code)
}
