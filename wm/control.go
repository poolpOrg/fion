package wm

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// fion listens on a socket of its own, for scripts, editors and Makefiles
// to post messages on the notification line, or drive it: fion msg and
// fion ctl talk to it. The socket is the user's alone, in
// $XDG_RUNTIME_DIR, or fion's state directory, named after the display.

// request is a line sent on the socket, in JSON.
type request struct {
	Cmd     string   `json:"cmd"` // msg, workspace, project, clear
	Level   string   `json:"level,omitempty"`
	Text    string   `json:"text,omitempty"`
	Timeout float64  `json:"timeout,omitempty"` // seconds, 0 for the level's default
	Args    []string `json:"args,omitempty"`
}

// socketPath is where the fion of display listens.
func socketPath(display string) string {
	name := "fion-" + strings.NewReplacer("/", "_", ":", "_").Replace(display) + ".sock"
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, name)
	}
	return filepath.Join(stateDir(), name)
}

// listenControl opens the socket, replacing a stale one, and serves it,
// handing each request to the event loop.
func (wm *Manager) listenControl(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// another fion may be using it
	if c, err := net.DialTimeout("unix", path, 200*time.Millisecond); err == nil {
		c.Close()
		return nil, fmt.Errorf("%s: in use", path)
	}
	os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	os.Chmod(path, 0o600)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go wm.serveControl(c)
		}
	}()
	return l, nil
}

func (wm *Manager) serveControl(c net.Conn) {
	defer c.Close()
	sc := bufio.NewScanner(c)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var req request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			fmt.Fprintf(c, "error: %v\n", err)
			continue
		}
		done := make(chan error, 1)
		wm.later <- func() { done <- wm.handleRequest(req) }
		if err := <-done; err != nil {
			fmt.Fprintf(c, "error: %v\n", err)
		} else {
			fmt.Fprintln(c, "ok")
		}
	}
}

// handleRequest runs a request, on the event loop.
func (wm *Manager) handleRequest(req request) error {
	switch req.Cmd {
	case "msg":
		lvl, err := parseLevel(req.Level)
		if err != nil {
			return err
		}
		wm.notify(lvl, req.Text, time.Duration(req.Timeout*float64(time.Second)))
	case "do":
		if len(req.Args) == 0 {
			return errors.New("do takes an action: fion ctl actions lists them")
		}
		a, args, err := parseAction(strings.Join(req.Args, " "))
		if err != nil {
			return err
		}
		return wm.runAction(a, args)
	case "clear", "workspace", "project":
		// the first commands, as actions
		name := map[string]string{"clear": "clear-messages"}[req.Cmd]
		if name == "" {
			name = req.Cmd
		}
		a, _ := findAction(name)
		return wm.runAction(a, req.Args)
	default:
		return fmt.Errorf("unknown command %q", req.Cmd)
	}
	return nil
}

// Control runs fion msg or fion ctl, with their arguments, against the fion
// of $DISPLAY, and reports whether args were one of them.
func Control(args []string, stdin io.Reader, stdout io.Writer) (bool, error) {
	if len(args) == 0 || (args[0] != "msg" && args[0] != "ctl") {
		return false, nil
	}
	if len(args) == 2 && args[0] == "ctl" && args[1] == "actions" {
		_, err := io.WriteString(stdout, actionList())
		return true, err
	}
	reqs, err := controlRequests(args, stdin)
	if err != nil {
		return true, err
	}
	c, err := net.Dial("unix", socketPath(os.Getenv("DISPLAY")))
	if err != nil {
		return true, fmt.Errorf("no fion on display %q: %v", os.Getenv("DISPLAY"), err)
	}
	defer c.Close()
	replies := bufio.NewScanner(c)
	for _, req := range reqs {
		b, _ := json.Marshal(req)
		if _, err := c.Write(append(b, '\n')); err != nil {
			return true, err
		}
		if !replies.Scan() {
			return true, errors.New("fion closed the connection")
		}
		if r := replies.Text(); r != "ok" {
			return true, errors.New(strings.TrimPrefix(r, "error: "))
		}
	}
	return true, nil
}

const controlUsage = `usage: fion msg [-l info|ok|warn|error] [-t seconds] [text ...]
       fion ctl do ACTION [ARGS ...]
       fion ctl actions
       fion ctl workspace next|prev|new|N
       fion ctl project NAME|DIR
       fion ctl clear
fion msg without text posts each line read from the standard input`

// controlRequests turns fion msg's and fion ctl's arguments into requests.
func controlRequests(args []string, stdin io.Reader) ([]request, error) {
	if args[0] == "ctl" {
		if len(args) < 2 {
			return nil, errors.New(controlUsage)
		}
		return []request{{Cmd: args[1], Args: args[2:]}}, nil
	}
	req := request{Cmd: "msg"}
	rest := args[1:]
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && rest[0] != "-" {
		if rest[0] == "--" {
			rest = rest[1:]
			break
		}
		if len(rest) < 2 {
			return nil, errors.New(controlUsage)
		}
		switch rest[0] {
		case "-l":
			if _, err := parseLevel(rest[1]); err != nil {
				return nil, err
			}
			req.Level = rest[1]
		case "-t":
			t, err := strconv.ParseFloat(rest[1], 64)
			if err != nil || t < 0 {
				return nil, fmt.Errorf("-t %s: not a number of seconds", rest[1])
			}
			req.Timeout = t
		default:
			return nil, errors.New(controlUsage)
		}
		rest = rest[2:]
	}
	if len(rest) > 0 {
		req.Text = strings.Join(rest, " ")
		return []request{req}, nil
	}
	var reqs []request
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		if line := strings.TrimRight(sc.Text(), "\r"); line != "" {
			r := req
			r.Text = line
			reqs = append(reqs, r)
		}
	}
	if len(reqs) == 0 {
		return nil, errors.New(controlUsage)
	}
	return reqs, sc.Err()
}
