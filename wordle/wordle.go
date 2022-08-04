package main

import (
	"bufio"
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	GREEN  = 32
	YELLOW = 33
)

var verbose = true

//go:embed 5letter_freq.txt
//go:embed corncob_lowercase.txt
var content embed.FS

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("usage: show <wordle-number>\n")
		os.Exit(1)
	}
	candidates, err := GetCandidates(os.Args[1])
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
	for _, c := range candidates {
		fmt.Printf("%v\n", c)
	}
}

type Candidate struct {
	Freq int
	Word string
}

func GetCandidates(guessFile string) ([]Candidate, error) {
	path := fmt.Sprintf("guesses/%s.txt", guessFile)
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	guesses, err := ParseGuesses(bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	err = NormalizeGuesses(guesses)
	if err != nil {
		return nil, err
	}
	colours := colourizeGuesses(guesses)
	fmt.Printf("%s\n", ANSIGuesses(guesses, colours))

	fmt.Printf("\nCandidates:\n")
	candidates, mustContain := findCandidates(guesses, colours)
	for i := range candidates {
		fmt.Printf("letter %d: ", i+1)
		for k := 'A'; k <= 'Z'; k++ {
			c := ' '
			if _, ok := candidates[i][k]; ok {
				c = k
			}
			fmt.Printf("%c", c)
		}
		fmt.Printf("\n")
	}
	fmt.Printf("unknown position: %s\n", mustContain)
	var regex string
	for i := range candidates {
		var s string
		for k := 'A'; k <= 'Z'; k++ {
			if _, ok := candidates[i][k]; ok {
				s += strings.ToLower(string(k))
			}
		}
		if len(s) > 1 {
			s = "[" + s + "]"
		}
		regex += s
	}
	if verbose {
		fmt.Printf("searching corncob list for %s\n", regex)
	}
	corncobMatches := mustGrep(regexp.MustCompile(`^`+regex+`$`), "corncob_lowercase.txt")
	regex = ""
	for _, match := range corncobMatches {
		if missingRequired(match, mustContain) {
			continue
		} else if verbose {
			fmt.Printf("corncob %s\n", match)
		}
		if len(regex) > 0 {
			regex += "|"
		}
		regex += match
	}
	if len(regex) == 0 {
		fmt.Printf("all matches eliminated\n")
		os.Exit(0)
	}
	if verbose {
		fmt.Printf("searching 5letter_freq list for %s\n", regex)
	}
	freqMatches := mustGrep(regexp.MustCompile(regex), "5letter_freq.txt")
	res := make([]Candidate, len(freqMatches))
	for i := range freqMatches {
		part := strings.Split(freqMatches[i], " ")
		if len(part) != 2 {
			return nil, fmt.Errorf("malformed match %d: '%s'", i, freqMatches[i])
		}
		freq, err := strconv.Atoi(part[0])
		if err != nil {
			return nil, fmt.Errorf("malformed match %d: freq '%s' is not a number", i, part[0])
		}
		res[i] = Candidate{
			Freq: freq,
			Word: part[1],
		}
	}
	return res, nil
}

func missingRequired(match, mustContain string) bool {
	for _, c := range mustContain {
		if !strings.Contains(match, strings.ToLower(string(c))) {
			if verbose {
				fmt.Printf("kicking out %s due to lack of %s\n", match, string(c))
			}
			return true
		}
	}
	return false
}

type Guess struct {
	Word   string
	Yellow string
	Green  string
}

func ParseGuesses(b []byte) ([]Guess, error) {
	scanner := bufio.NewScanner(bytes.NewReader(b))
	n := 0
	var guesses []Guess
	for scanner.Scan() {
		n++
		l := scanner.Text()
		if strings.HasPrefix(l, "#") {
			continue
		}
		if len(l) == 0 {
			continue
		}
		cols := strings.Split(l, "\t")
		var word, yellow, green string
		word = cols[0]
		if len(cols) > 1 {
			yellow = cols[1]
		}
		if len(cols) > 2 {
			green = cols[2]
		}
		guesses = append(guesses, Guess{Word: word, Yellow: yellow, Green: green})
	}
	return guesses, nil
}

func NormalizeGuesses(guesses []Guess) error {
	for i := range guesses {
		g := &guesses[i]
		g.Word = strings.ToUpper(g.Word)
		g.Yellow = strings.ToUpper(g.Yellow)
		if err := checkTemplate(g.Word, g.Yellow); err != nil {
			return fmt.Errorf("guess %d: yellow %w", i+1, err)
		}
		g.Green = strings.ToUpper(g.Green)
		if err := checkTemplate(g.Word, g.Green); err != nil {
			return fmt.Errorf("guess %d: green %w", i+1, err)
		}
	}
	return nil
}

func checkTemplate(word, template string) error {
	if len(template) > len(word) {
		return errors.New("template is longer than guess")
	}
	for i, c := range template {
		if c >= 'A' && c <= 'Z' && c != rune(word[i]) {
			return fmt.Errorf("template and word letter %d mismatch, %c vs %c", i+1, template[i], word[i])
		}
	}
	return nil
}

func colourizeGuesses(guesses []Guess) [][]int {
	colours := make([][]int, len(guesses))
	for i, g := range guesses {
		c := make([]int, len(g.Word))
		applyColours(c, g.Word, g.Yellow, YELLOW)
		applyColours(c, g.Word, g.Green, GREEN)
		colours[i] = c
	}
	return colours
}

func ANSIGuesses(guesses []Guess, colours [][]int) string {
	var res string
	for i := range guesses {
		res += colourizeWord(guesses[i].Word, colours[i]) + "\n"
	}
	return res
}

func applyColours(c []int, word, template string, colour int) {
	for i := range word {
		if len(template) <= i {
			break
		}
		if template[i] >= 'A' && template[i] <= 'Z' && template[i] == word[i] {
			c[i] = colour
		}
	}
}

func colourizeWord(word string, colours []int) string {
	var cur int
	var res string
	for i := range word {
		if colours[i] != cur {
			if colours[i] > 0 {
				res += ansi(colours[i])
			} else {
				res += ansiOff()
			}
			cur = colours[i]
		}
		res += string(word[i])
	}
	if cur != 0 {
		res += ansiOff()
	}
	return res
}

func ansi(c int) string { return "\033[" + fmt.Sprintf("%d", c) + "m" }

func ansiOff() string { return "\033[0m" }

func findCandidates(guesses []Guess, colours [][]int) ([]map[rune]struct{}, string) {
	candidates := make([]map[rune]struct{}, 5)
	var mustContain string
	for i := 0; i < 5; i++ {
		candidates[i] = allLetters()
	}
	for i := range guesses {
		word := guesses[i].Word
		for j, c := range colours[i] {
			letter := rune(word[j])
			switch c {
			case 0:
				if alreadyApplied(word, letter) {
					delete(candidates[j], letter)
				} else {
					for n, m := range candidates {
						if verbose {
							fmt.Printf("removing %c for position %d\n", letter, n)
						}
						delete(m, letter)
					}
				}
			case YELLOW:
				if verbose {
					fmt.Printf("removing %c for position %d\n", letter, j)
				}
				delete(candidates[j], letter)
				if !strings.Contains(mustContain, string(letter)) {
					mustContain += string(letter)
				}
			case GREEN:
				candidates[j] = map[rune]struct{}{letter: struct{}{}}
			}
		}
	}
	return candidates, mustContain
}

func alreadyApplied(word string, letter rune) bool {
	return strings.Count(word, string(letter)) > 1
}
func allLetters() map[rune]struct{} {
	res := make(map[rune]struct{})
	for c := 'A'; c <= 'Z'; c++ {
		res[c] = struct{}{}
	}
	return res
}

func mustGrep(exp *regexp.Regexp, path string) []string {
	var res []string
	f, err := content.ReadFile(path)
	if err != nil {
		panic(fmt.Errorf("embedded %s: %w", path, err))
	}
	scanner := bufio.NewScanner(bytes.NewReader(f))
	for scanner.Scan() {
		word := scanner.Text()
		if exp.MatchString(word) {
			res = append(res, word)
		}
	}
	return res
}
