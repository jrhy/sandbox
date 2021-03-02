package main

import (
	rpio "github.com/stianeikeland/go-rpio"
	"os"
	"time"
)

func main() {
	err := rpio.Open()
	if err != nil {
		panic(err)
	}
	defer rpio.Close()
	ch := rpio.Pin(21)
	//ch2 := rpio.Pin(20)
	//ch3 := rpio.Pin(21)

	ch.Mode(rpio.Output)
	if len(os.Args) == 2 {
		if os.Args[1] == "on" {
			ch.Low()
		} else {
			ch.High()
		}
	} else {
		ch.Low()
		time.Sleep(30 * time.Millisecond)
		ch.High()
	}
}
