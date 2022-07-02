package main

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
		// TODO 377 is missing pinto because it's not on the corncob list
		{"orig1", "retro"},
		{"orig2", "sissy"},
		{"375", "gawky"},
		{"beady", "beady"},
		{"epoxy", "epoxy"},
		{"nymph", "nymph"},
		{"stove", "stove"},
		{"tacit", "tacit"},
		{"watch", "watch"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.path, func(t *testing.T) {
			t.Parallel()
			candidates, err := GetCandidates(c.path)
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
