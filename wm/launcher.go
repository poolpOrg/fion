package wm

import (
	"path/filepath"
	"sort"
	"strings"
)

// The launcher, on Mod+Return, is a prompt in the manner of macOS's
// Spotlight: typing narrows down the commands in $PATH, the desktop
// applications and the command lines run before, and Return runs the
// selection or what was typed, through the shell. This file holds its
// model, independent of X.

type launchKind int

const (
	kindCommand launchKind = iota // an executable in $PATH
	kindApp                       // a desktop application
	kindHistory                   // a command line run before
)

type launchItem struct {
	label   string // what is shown and matched
	command string // what runs, matched too
	kind    launchKind
	uses    int // times it was run, from the history
}

type matchClass int

const (
	noMatch matchClass = iota
	fuzzyMatch
	substringMatch
	prefixMatch
)

// match rates how name matches query, regardless of case: a class, then a
// score within the class where higher is better.
func match(query, name string) (matchClass, int) {
	q, n := strings.ToLower(query), strings.ToLower(name)
	if q == "" {
		return prefixMatch, 0
	}
	if strings.HasPrefix(n, q) {
		return prefixMatch, -len(n)
	}
	if i := strings.Index(n, q); i >= 0 {
		return substringMatch, -4*i - len(n)
	}

	// the query's letters in order, anywhere
	gaps, last, j := 0, -1, 0
	for i := 0; i < len(n) && j < len(q); i++ {
		if n[i] != q[j] {
			continue
		}
		if last >= 0 {
			gaps += i - last - 1
		}
		last, j = i, j+1
	}
	if j < len(q) {
		return noMatch, 0
	}
	return fuzzyMatch, -4*gaps - len(n)
}

// matchItem rates the best match among an item's label and the program its
// command runs.
func matchItem(query string, it launchItem) (matchClass, int) {
	class, score := match(query, it.label)
	program := filepath.Base(strings.SplitN(it.command, " ", 2)[0])
	if c, s := match(query, program); c > class || (c == class && s > score) {
		class, score = c, s
	}
	return class, score
}

type launcher struct {
	items []launchItem

	query    string
	matches  []int // into items, best first
	selected int   // into matches
	moved    bool  // the selection was moved since the query changed
}

func newLauncher(items []launchItem) *launcher {
	l := &launcher{items: items}
	l.update()
	return l
}

func (l *launcher) setQuery(q string) {
	l.query = q
	l.update()
}

// update ranks the items matching the query. With an empty query, the
// command lines run before, most used first, or when there are none, the
// applications.
func (l *launcher) update() {
	type ranked struct {
		i     int
		class matchClass
		score int
	}
	var rs []ranked
	q := strings.TrimSpace(l.query)
	if q == "" {
		for i, it := range l.items {
			if it.uses > 0 {
				rs = append(rs, ranked{i: i})
			}
		}
		if len(rs) == 0 {
			for i, it := range l.items {
				if it.kind == kindApp {
					rs = append(rs, ranked{i: i})
				}
			}
		}
	} else {
		for i, it := range l.items {
			if class, score := matchItem(q, it); class != noMatch {
				rs = append(rs, ranked{i, class, score})
			}
		}
	}

	sort.SliceStable(rs, func(a, b int) bool {
		ra, rb := rs[a], rs[b]
		ia, ib := l.items[ra.i], l.items[rb.i]
		switch {
		case ra.class != rb.class:
			return ra.class > rb.class
		case ia.uses != ib.uses:
			return ia.uses > ib.uses
		case ra.score != rb.score:
			return ra.score > rb.score
		}
		return strings.ToLower(ia.label) < strings.ToLower(ib.label)
	})

	l.matches = l.matches[:0]
	for _, r := range rs {
		l.matches = append(l.matches, r.i)
	}
	l.selected, l.moved = 0, false
}

// move moves the selection by delta, wrapping around.
func (l *launcher) move(delta int) {
	if len(l.matches) == 0 {
		return
	}
	l.selected = (l.selected + delta + len(l.matches)) % len(l.matches)
	l.moved = true
}

// selection returns the selected item, if any.
func (l *launcher) selection() (launchItem, bool) {
	if len(l.matches) == 0 {
		return launchItem{}, false
	}
	return l.items[l.matches[l.selected]], true
}

// complete replaces the query with the selected command, to add arguments.
func (l *launcher) complete() {
	if it, ok := l.selection(); ok {
		l.setQuery(it.command + " ")
	}
}

// backspace removes the last character of the query.
func (l *launcher) backspace() {
	if l.query != "" {
		l.setQuery(l.query[:len(l.query)-1])
	}
}

// deleteWord removes the last word of the query.
func (l *launcher) deleteWord() {
	q := strings.TrimRight(l.query, " ")
	if i := strings.LastIndexByte(q, ' '); i >= 0 {
		l.setQuery(q[:i+1])
	} else {
		l.setQuery("")
	}
}

// line is the command line Return runs: the selected item's command, or
// what was typed when it has arguments, unless the selection was moved to,
// or when nothing matches.
func (l *launcher) line() string {
	q := strings.TrimSpace(l.query)
	it, ok := l.selection()
	if !ok || (strings.ContainsRune(q, ' ') && !l.moved) {
		return q
	}
	return it.command
}
