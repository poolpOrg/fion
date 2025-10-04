package main

import (
	"log"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
}

func main() {
	wm, err := NewWM()
	if err != nil {
		log.Fatalf("init WM: %v", err)
	}
	defer wm.Close()

	if err := wm.Run(); err != nil {
		log.Fatal(err)
	}
}
