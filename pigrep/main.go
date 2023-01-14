package main

import (
	"fmt"
	"os"
	"strconv"

	pigen "github.com/dimchansky/pi-gen-go"
)

func main() {
	if len(os.Args) == 1 {
		fmt.Printf("usage: pigrep <digits>\n")
		os.Exit(1)
	}
	pattern := os.Args[1]
	_, err := strconv.Atoi(pattern)
	if err != nil {
		fmt.Printf("usage: pigrep <digit-pattern>\n")
		os.Exit(1)
	}

	var s string
	var n int
	start := GetStartOfPattern(pattern, func(c byte) {
		s += string(c + '0')
		n++
		if n%1000 == 0 {
			fmt.Printf("\r%d digits...", n)
		}
	})
	fmt.Printf("\n3.%s\n", s[1:])
	fmt.Printf("start digit: %d\n", start)
}

func GetStartOfPattern(pattern string, cb func(byte)) int {
	var digits string
	var n int
	g := pigen.New()
	for {
		n++
		nextdig := byte(g.NextDigit())
		if cb != nil {
			cb(nextdig)
		}
		digits += string(nextdig + '0')
		if len(digits) > len(pattern) {
			digits = digits[1:]
		}
		if digits == pattern {
			return n + 1 - len(pattern)
		}
	}
}
