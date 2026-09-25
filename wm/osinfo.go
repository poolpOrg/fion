package wm

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// The info bar shows the operating system fion runs on: its name and
// version, as the system names them, and the machine's architecture.

type osInfo struct {
	kind    string // what the icon is drawn for: openbsd, linux, ...
	name    string
	version string
	machine string
}

// String is how the info bar shows it: OpenBSD/7.9 (arm64).
func (o osInfo) String() string {
	s := o.name
	if o.version != "" {
		s += "/" + o.version
	}
	if o.machine != "" {
		s += " (" + o.machine + ")"
	}
	return s
}

var (
	osOnce sync.Once
	osSelf osInfo
)

// thisOS detects the operating system, once.
func thisOS() osInfo {
	osOnce.Do(func() {
		osSelf = detectOS()
	})
	return osSelf
}

func detectOS() osInfo {
	o := osInfo{kind: runtime.GOOS}
	var u unix.Utsname
	if err := unix.Uname(&u); err == nil {
		o.name = unix.ByteSliceToString(u.Sysname[:])
		o.version = unix.ByteSliceToString(u.Release[:])
		o.machine = unix.ByteSliceToString(u.Machine[:])
	}
	if o.machine == "" {
		o.machine = runtime.GOARCH
	}

	switch runtime.GOOS {
	case "linux":
		// the distribution, rather than the kernel
		if f, err := os.Open("/etc/os-release"); err == nil {
			defer f.Close()
			if name, version := parseOSRelease(f); name != "" {
				o.name, o.version = name, version
			}
		}
	case "darwin":
		o.name, o.version = "macOS", ""
		if out, err := exec.Command("sw_vers", "-productVersion").Output(); err == nil {
			o.version = strings.TrimSpace(string(out))
		}
	}
	return o
}

// parseOSRelease returns a distribution's name and version from its
// os-release file, the name shortened of its GNU/Linux or Linux.
func parseOSRelease(r io.Reader) (name, version string) {
	values := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		values[key] = strings.Trim(value, `"'`)
	}

	name = values["NAME"]
	for _, suffix := range []string{" GNU/Linux", " Linux"} {
		name = strings.TrimSuffix(name, suffix)
	}
	if name == "" {
		name = values["ID"]
	}
	return name, values["VERSION_ID"]
}
