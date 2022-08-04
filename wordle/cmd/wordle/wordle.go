package main

import (
	"fmt"
	"os"

	"github.com/jrhy/sandbox/wordle"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("usage: show <wordle-number>\n")
		os.Exit(1)
	}
	guesses, err := wordle.LoadGuesses(os.Args[1])
	if err != nil {
		fmt.Printf("%s: %v\n", os.Args[1], err)
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
