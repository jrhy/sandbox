package wordle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSamples(t *testing.T) {
	t.Parallel()
	type testCase struct {
		path string
		word string
	}
	cases := []testCase{
		{"guesses/orig1.txt", "retro"},
		{"guesses/orig2.txt", "sissy"},
		//{"guesses/375.txt", "gawky"},
		{"guesses/beady.txt", "beady"},
		{"guesses/epoxy.txt", "epoxy"},
		{"guesses/nymph.txt", "nymph"},
		{"guesses/stove.txt", "stove"},
		{"guesses/tacit.txt", "tacit"},
		{"guesses/watch.txt", "watch"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.path, func(t *testing.T) {
			t.Parallel()
			guesses, err := LoadGuesses(c.path)
			require.NoError(t, err)
			candidates, err := GetCandidates(guesses)
			require.NoError(t, err)
			require.Contains(t, words(candidates), c.word)
		})
	}
}

func words(c []Candidate) []string {
	res := make([]string, len(c))
	for i := range c {
		res[i] = c[i].Word
	}
	return res
}
