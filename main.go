package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"syscall"

	fion "github.com/poolpOrg/fion/wm"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
}

func main() {
	// fion msg and fion ctl talk to the fion running
	if ok, err := fion.Control(os.Args[1:], os.Stdin, os.Stdout); ok {
		if err != nil {
			fmt.Fprintln(os.Stderr, "fion:", err)
			os.Exit(1)
		}
		return
	}
	wm, err := fion.NewManager()
	if err != nil {
		log.Fatalf("init WM: %v", err)
	}
	err = wm.Run()
	wm.Close()
	var restart *fion.Restart
	if errors.As(err, &restart) {
		// again, the binary installed now, with the layout written down
		exe, err := os.Executable()
		if err != nil {
			log.Fatalf("restart: %v", err)
		}
		env := append(slices.DeleteFunc(os.Environ(), func(v string) bool {
			return strings.HasPrefix(v, "FION_RESTORE=")
		}), "FION_RESTORE="+restart.Path)
		log.Fatalf("restart: %v", syscall.Exec(exe, os.Args, env))
	}
	if err != nil {
		log.Fatal(err)
	}
}
