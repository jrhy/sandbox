package main

import (
	"C"
	"unsafe"

	"github.com/jrhy/sandbox/s3db-rs/go_from_rust/gob2json"
)

func main() {}

//export ReadRoot
func ReadRoot(p unsafe.Pointer, l C.int) *C.char {
	j, err := gob2json.ReadRoot(C.GoBytes(p, l))
	if err != nil {
		return nil
	}
	return C.CString(j)
}
