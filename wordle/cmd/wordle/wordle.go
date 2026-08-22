package main

import (
	"fmt"
	"io/ioutil"
	"os"

	flags "github.com/jessevdk/go-flags"

	wordle "github.com/jrhy/sandbox/wordle/core"
)

type Flags struct {
	GuessFile  string   `short:"f"`
	TargetWord string   `long:"target" short:"t"`
	Guesses    []string `short:"g"`
	Verbose    bool     `short:"v"`
}

func main() {
	var f Flags
	p := flags.NewParser(&f, 0)
	_, err := p.ParseArgs(os.Args)
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
	if f.GuessFile != "" && f.TargetWord != "" ||
		f.GuessFile == "" && f.TargetWord == "" {
		fmt.Printf("usage: wordle -f <guessfile>\n")
		fmt.Printf("usage: wordle -t/--target word -g guess...\n")
		os.Exit(1)
	}
	if f.Verbose {
		wordle.Verbose = true
	}
	var guesses []wordle.Guess
	if f.GuessFile != "" {
		guesses, err = loadGuesses(f.GuessFile)
	} else if f.TargetWord != "" {
		guesses, err = wordle.GuessesForTarget(f.TargetWord, f.Guesses)
	}
	if err != nil {
		fmt.Printf("%s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	colours := wordle.ColourizeGuesses(guesses)
	fmt.Printf("%s\n", wordle.ANSIGuesses(guesses, colours))
	candidates, err := wordle.GetCandidates(guesses)
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
	for _, c := range candidates {
		fmt.Printf("%v\n", c)
	}
}

func loadGuesses(path string) ([]wordle.Guess, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	guesses, err := wordle.ParseGuesses(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return guesses, wordle.NormalizeGuesses(guesses)
}
