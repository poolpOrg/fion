package wm

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseFstatListening(t *testing.T) {
	out := `USER     CMD          PID   FD MOUNT        INUM  MODE         R/W    SZ|DV
root     sshd       75629    3* internet stream tcp 0x0 *:22
root     sshd       75629    4* internet6 stream tcp 0x0 [::]:22
gilles   node        1234   20* internet stream tcp 0x0 127.0.0.1:3000
gilles   ssh         4321    3* internet stream tcp 0x0 10.0.0.2:5000 --> 10.0.0.1:22
root     unbound       99    5* internet dgram udp 0x0 127.0.0.1:53
`
	got := dedupePorts(parseFstatListening(strings.NewReader(out)))
	if len(got) != 2 || got[0].port != 22 || got[0].addr != "*" || got[0].name != "sshd" || got[0].pid != 75629 ||
		got[1].port != 3000 || got[1].addr != "127.0.0.1" {
		t.Fatalf("ports %+v", got)
	}
}

func TestTopProcesses(t *testing.T) {
	var s procSampler
	s.top(5)
	// something to measure: this process, busy
	end := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(end) {
	}
	top := s.top(50)
	found := false
	for _, u := range top {
		if u.pid == int32(os.Getpid()) {
			found = u.cpu > 10
		}
	}
	if !found {
		t.Fatalf("this busy process isn't among the top ones: %+v", top)
	}
}
