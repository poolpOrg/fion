package wm

import (
	"sort"
	"strconv"
)

// The panel keeps the last values of what it shows, one a second while it
// is shown, for its graphs.

const historyLen = 600 // ten minutes

// series is a value's history, oldest first.
type series struct {
	vals []float64
}

func (s *series) push(v float64) {
	s.vals = append(s.vals, v)
	if len(s.vals) > historyLen {
		s.vals = s.vals[len(s.vals)-historyLen:]
	}
}

// last returns the n last values, oldest first.
func (s *series) last(n int) []float64 {
	if s == nil {
		return nil
	}
	if n < len(s.vals) {
		return s.vals[len(s.vals)-n:]
	}
	return s.vals
}

type histories map[string]*series

func (h histories) push(key string, v float64) {
	s, ok := h[key]
	if !ok {
		s = &series{}
		h[key] = s
	}
	s.push(v)
}

// record keeps a snapshot's values.
func (h histories) record(s sysSnapshot) {
	h.push("cpu", s.cpuTotal)
	for i, v := range s.cpus {
		h.push(cpuKey(i), v)
	}
	h.push("load", s.load[0])
	if s.mem.total > 0 {
		h.push("mem", percent(s.mem.used, s.mem.total))
	}
	if s.mem.swapTotal > 0 {
		h.push("swap", percent(s.mem.swapUsed, s.mem.swapTotal))
	}
	for _, r := range s.disks {
		h.push("disk:"+r.name+":in", r.in)
		h.push("disk:"+r.name+":out", r.out)
	}
	for _, r := range s.nets {
		h.push("net:"+r.name+":in", r.in)
		h.push("net:"+r.name+":out", r.out)
	}
	for _, t := range s.allSensors {
		h.push("temp:"+t.name, t.temp)
	}
}

func cpuKey(i int) string { return "cpu" + strconv.Itoa(i) }

// graphColumns scales the last values to heights of at most h pixels, one
// every step pixels over w, against top, or their peak when top is 0. It
// returns the heights, oldest first, and the scale's top.
func graphColumns(vals []float64, w, h, step int, top float64) ([]int, float64) {
	n := max(1, w/step)
	if len(vals) > n {
		vals = vals[len(vals)-n:]
	}
	if top <= 0 {
		for _, v := range vals {
			top = max(top, v)
		}
	}
	cols := make([]int, len(vals))
	if top <= 0 {
		return cols, 0
	}
	for i, v := range vals {
		cols[i] = min(h, max(0, int(v/top*float64(h)+0.5)))
	}
	return cols, top
}

// ioNames are the names of the disks or interfaces in totals, sorted:
// all of them, idle or not, so that they stay in place.
func ioNames(totals map[string][2]uint64) []string {
	names := make([]string, 0, len(totals))
	for n := range totals {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// rateOf returns the rate of name among rs, and whether it has one yet.
func rateOf(rs []ioRate, name string) (ioRate, bool) {
	for _, r := range rs {
		if r.name == name {
			return r, true
		}
	}
	return ioRate{name: name}, false
}
