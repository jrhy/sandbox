package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHappy(t *testing.T) {
	x, err := CalibrationLine("treb7uchet")
	require.NoError(t, err)
	require.Equal(t, 77, x)
}

func TestPartTwo(t *testing.T) {
	x, err := CalibrationLine("abcone2threexyz")
	require.NoError(t, err)
	require.Equal(t, 13, x)
	lines := []string{
		"two1nine",
		"eightwothree",
		"abcone2threexyz",
		"xtwone3four",
		"4nineeightseven2",
		"zoneight234",
		"7pqrstsixteen",
	}
	expected := []int{29, 83, 13, 24, 42, 14, 76}
	sum := 0
	for i, l := range lines {
		x, err := CalibrationLine(l)
		require.NoError(t, err)
		require.Equal(t, expected[i], x, l)
		sum += x
	}
	require.Equal(t, 281, sum)
}

func TestError(t *testing.T) {
	_, err := CalibrationLine("")
	require.Contains(t, err.Error(), "no first digit")
}
