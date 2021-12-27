package main

/*
typedef void(*NotificationListener)(void);
extern void go_callback(const void *cb);
typedef void(*NotificationParamListener)(long);
extern void go_callback2(const void *cb, long l);
#cgo LDFLAGS: -L. -ltoscala
*/
import "C"

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"unsafe"
)

var count int
var mtx sync.Mutex

//export Add
func Add(a, b int) int { return a + b }

//export Cosine
func Cosine(x float64) float64 { return math.Cos(x) }

//export Sort
func Sort(vals []int) { sort.Ints(vals) }

//export CallMe
func CallMe(cb C.NotificationListener) {
	fmt.Println("about to call!")
	(C.go_callback)(unsafe.Pointer(cb))
}

//export DoNothingWithArg
func DoNothingWithArg(o *C.void) {
	fmt.Printf("from go: i am in DoNothingWithArg, o=%T %o %v\n", o, o, o)
	u := unsafe.Pointer(o)
	fmt.Printf("from go: i am in DoNothingWithArg, u=%T %o %v\n", u, u, u)
}

//export CallMeWithSomething
func CallMeWithSomething(cb C.NotificationParamListener, l C.long) {
	fmt.Printf("about to call with param! %v\n", l)
	(C.go_callback2)(unsafe.Pointer(cb), l)
}

//export Log
func Log(msg string) int {
	mtx.Lock()
	defer mtx.Unlock()
	fmt.Println(msg)
	count++
	return count
}

func main() {}
