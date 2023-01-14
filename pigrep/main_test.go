package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Happy(t *testing.T) {
	var s string
	assert.Equal(t, 2, GetStartOfPattern("14159", func(c byte) { s += string(c + '0') }))
	assert.Equal(t, s, "314159")
}
