package wm

import (
	"bufio"
	"io"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// What the panel's CPU and Ports views show about processes: those using
// the most CPU, and the TCP ports listened to, with their process. Both
// are slow to gather, lsof on macOS, the whole process table everywhere,
// so they are sampled away from the event loop, every two seconds, only
// while shown.

type procUsage struct {
	pid  int32
	name string
	cpu  float64 // percent of a CPU, over the last sample
	rss  uint64
}

type listenPort struct {
	addr string // as the system shows it, * for any
	port uint32
	pid  int32 // 0 when unknown
	name string
}

// procSampler keeps the CPU times of the processes, to turn the next
// sample into percentages.
type procSampler struct {
	at    time.Time
	times map[int32]float64
}

// top returns the n processes using the most CPU since the last call.
func (s *procSampler) top(n int) []procUsage {
	procs, err := process.Processes()
	if err != nil {
		return nil
	}
	now := time.Now()
	times := make(map[int32]float64, len(procs))
	var out []procUsage
	dt := now.Sub(s.at).Seconds()
	for _, p := range procs {
		t, err := p.Times()
		if err != nil {
			continue
		}
		total := t.User + t.System
		times[p.Pid] = total
		prev, ok := s.times[p.Pid]
		if !ok || s.at.IsZero() || dt <= 0 {
			continue
		}
		u := procUsage{pid: p.Pid, cpu: max(0, (total-prev)/dt*100)}
		out = append(out, u)
	}
	s.at, s.times = now, times
	sort.Slice(out, func(i, j int) bool { return out[i].cpu > out[j].cpu })
	if len(out) > n {
		out = out[:n]
	}
	// names and memory for those shown only
	for i := range out {
		if p, err := process.NewProcess(out[i].pid); err == nil {
			out[i].name, _ = p.Name()
			if m, err := p.MemoryInfo(); err == nil {
				out[i].rss = m.RSS
			}
		}
	}
	return out
}

// listeningPorts returns the TCP ports listened to, by port.
func listeningPorts() []listenPort {
	var out []listenPort
	if runtime.GOOS == "openbsd" {
		// netstat doesn't tell the processes there, fstat does
		if b, err := exec.Command("fstat").Output(); err == nil {
			out = parseFstatListening(strings.NewReader(string(b)))
		}
	} else if conns, err := net.Connections("tcp"); err == nil {
		names := map[int32]string{}
		for _, c := range conns {
			if c.Status != "LISTEN" {
				continue
			}
			lp := listenPort{addr: c.Laddr.IP, port: c.Laddr.Port, pid: c.Pid}
			if lp.addr == "" || lp.addr == "0.0.0.0" || lp.addr == "::" {
				lp.addr = "*"
			}
			if c.Pid > 0 {
				name, ok := names[c.Pid]
				if !ok {
					if p, err := process.NewProcess(c.Pid); err == nil {
						name, _ = p.Name()
					}
					names[c.Pid] = name
				}
				lp.name = name
			}
			out = append(out, lp)
		}
	}
	return dedupePorts(out)
}

// dedupePorts keeps one line per port and process, sorted by port: a
// server listens on IPv4 and IPv6 alike.
func dedupePorts(ps []listenPort) []listenPort {
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].port != ps[j].port {
			return ps[i].port < ps[j].port
		}
		return ps[i].pid < ps[j].pid
	})
	var out []listenPort
	for _, p := range ps {
		if n := len(out); n > 0 && out[n-1].port == p.port && out[n-1].pid == p.pid {
			if out[n-1].addr != p.addr {
				out[n-1].addr = "*"
			}
			continue
		}
		out = append(out, p)
	}
	return out
}

// parseFstatListening reads the TCP sockets listening in the output of
// OpenBSD's fstat: those with a local address and no peer.
//
//	root     sshd       75629    3* internet stream tcp 0x0 *:22
func parseFstatListening(r io.Reader) []listenPort {
	var out []listenPort
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 9 || !strings.HasPrefix(f[4], "internet") || f[5] != "stream" || f[6] != "tcp" {
			continue
		}
		if len(f) > 9 {
			// connected: <-- or --> a peer
			continue
		}
		addr := f[8]
		i := strings.LastIndex(addr, ":")
		if i < 0 {
			continue
		}
		port, err := strconv.ParseUint(addr[i+1:], 10, 32)
		if err != nil || port == 0 {
			continue
		}
		host := strings.Trim(addr[:i], "[]")
		pid, _ := strconv.ParseInt(f[2], 10, 32)
		out = append(out, listenPort{addr: host, port: uint32(port), pid: int32(pid), name: f[1]})
	}
	return out
}
