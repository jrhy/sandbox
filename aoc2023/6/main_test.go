package fun

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const IntroInput = `Time:      7  15   30
Distance:  9  40  200`

var tRE = regexp.MustCompile(`^Time:.*\n`)
var dRE = regexp.MustCompile(`^Distance:.*`)
var numberRE = regexp.MustCompile(` +(\d+)`)

func ScanRaces(s string) ([]int64, []int64) {
	r := tRE.FindString(s)
	if r == "" {
		panic(r)
	}
	s = s[len(r):]
	rs := numberRE.FindAllStringSubmatch(r[5:], -1)
	ns := make([]int64, len(rs))
	for i := 0; i < len(rs); i++ {
		ns[i] = MustParse(rs[i][1])
	}
	t := ns

	r = dRE.FindString(s)
	if r == "" {
		panic(s)
	}
	s = s[len(r):]
	rs = numberRE.FindAllStringSubmatch(r[9:], -1)
	ns = make([]int64, len(rs))
	for i := 0; i < len(rs); i++ {
		ns[i] = MustParse(rs[i][1])
	}
	d := ns

	return t, d
}

func MustParse(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic(err)
	}
	return n
}

func WaysToWin(t, d int64) int64 {
	min, max := int64(1<<62), int64(0)
	for i := int64(0); i < t; i++ {
		v := i
		md := v * (t - i)
		if md > d { // win
			if i < min {
				min = i
			}
			if i > max {
				max = i
			}
		}
	}
	if min == 1000 {
		return 0
	}
	return max - min + 1
}

func AllWaysToWin(t, d []int64) []int64 {
	res := make([]int64, len(t))
	for i := range t {
		res[i] = WaysToWin(t[i], d[i])
	}
	return res
}
func Mult(n []int64) int64 {
	var res int64 = 1
	for i := range n {
		res *= n[i]
	}
	return res
}

func TestIntro(t *testing.T) {
	times, distances := ScanRaces(IntroInput)
	assert.Equal(t, []int64{7, 15, 30}, times)
	assert.Equal(t, []int64{9, 40, 200}, distances)
	assert.Equal(t, int64(4), WaysToWin(7, 9))
	assert.Equal(t, int64(8), WaysToWin(15, 40))
	assert.Equal(t, int64(9), WaysToWin(30, 200))
	assert.Equal(t, []int64{4, 8, 9}, AllWaysToWin(times, distances))
	assert.Equal(t, int64(288), Mult(AllWaysToWin(times, distances)))
}

func Sum(pns []int) int {
	sum := 0
	for _, i := range pns {
		sum += i
	}
	return sum
}

const AOCInput = `Time:        54     70     82     75
Distance:   239   1142   1295   1253`

func TestAOC(t *testing.T) {
	times, distances := ScanRaces(AOCInput)
	fmt.Println(Mult(AllWaysToWin(times, distances)))
}

const Part2Intro = `Time:      71530
Distance:  940200`

func TestPart2Intro(t *testing.T) {
	times, distances := ScanRaces(Part2Intro)
	require.Equal(t, int64(71503), WaysToWin(times[0], distances[0]))
}

const Part2Input = `Time:        54708275
Distance:   239114212951253`

func TestPart2(t *testing.T) {
	times, distances := ScanRaces(Part2Input)
	fmt.Println(Mult(AllWaysToWin(times, distances)))
}
