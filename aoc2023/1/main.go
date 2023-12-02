package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
)

var FirstDigit = regexp.MustCompile(`([1-9]|one|two|three|four|five|six|seven|eight|nine)`)
var LastDigit = regexp.MustCompile(`.*([1-9]|one|two|three|four|five|six|seven|eight|nine)`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	path := "calibration-input.txt"
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	scanner := bufio.NewScanner(file)
	// optionally, resize scanner's capacity for lines over 64K, see next example
	sum := 0
	for scanner.Scan() {
		fmt.Println(scanner.Text())
		x, err := CalibrationLine(scanner.Text())
		if err != nil {
			return fmt.Errorf("word: %s: %v", scanner.Text(), err)
		}
		sum += x
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner: %w", err)
	}

	fmt.Printf("%d\n", sum)
	return nil
}

func WordToDigit(s string) int64 {
	switch s {
	case "one":
		return 1
	case "two":
		return 2
	case "three":
		return 3
	case "four":
		return 4
	case "five":
		return 5
	case "six":
		return 6
	case "seven":
		return 7
	case "eight":
		return 8
	case "nine":
		return 9
	}
	return 0
}

func CalibrationLine(s string) (int, error) {
	first := FirstDigit.FindStringSubmatch(s)
	last := LastDigit.FindStringSubmatch(s)
	if len(first) < 2 || first[1] == "" {
		return 0, errors.New("no first digit")
	}
	if len(last) < 2 || last[1] == "" {
		return 0, errors.New("no last digit")
	}
	var err error
	var f, l int64
	f = WordToDigit(first[1])
	if f == 0 {
		f, err = strconv.ParseInt(first[1], 10, 32)
		if err != nil {
			return 0, fmt.Errorf("first: %w", err)
		}
	}
	l = WordToDigit(last[1])
	if l == 0 {
		l, err = strconv.ParseInt(last[1], 10, 32)
		if err != nil {
			return 0, fmt.Errorf("last: %w", err)
		}
	}
	return int(f*10 + l), nil
}
