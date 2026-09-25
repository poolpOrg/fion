package main

import (
	"fmt"
	"log"
	"os"

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
	defer wm.Close()

	if err := wm.Run(); err != nil {
		log.Fatal(err)
	}
}
