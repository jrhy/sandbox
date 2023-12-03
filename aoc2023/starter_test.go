package fun

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

const IntroInput = ``

var mapBeginningRE = regexp.MustCompile(`^(.*) map:\n`)
var tripleRE = regexp.MustCompile(`^(\d+) (\d+) (\d+)\n`)

func MustParseInt(s string) int {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic(s + ": " + err.Error())
	}
	return int(n)
}

func TestIntro(t *testing.T) {
	assert.True(t, true)
}

const AOCInput = ``

func TestAOC(t *testing.T) {
	fmt.Println()
}

func TestPart2Intro(t *testing.T) {
	assert.True(t, true)
}

func TestPart2(t *testing.T) {
	fmt.Println()
}

func Sum(pns []int) int {
	sum := 0
	for _, i := range pns {
		sum += i
	}
	return sum
}

func Min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}
