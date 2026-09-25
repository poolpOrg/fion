package main

import (
	"log"

	fion "github.com/poolpOrg/fion/wm"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
}

func main() {
	wm, err := fion.NewManager()
	if err != nil {
		log.Fatalf("init WM: %v", err)
	}
	defer wm.Close()

	if err := wm.Run(); err != nil {
		log.Fatal(err)
	}
}
