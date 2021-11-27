package main

import (
	"C"
	"log"
	"unsafe"

	"github.com/jrhy/sandbox/s3db-rs/go_from_rust/gob2json"
)

func main() {}

//export ReadNode
func ReadNode(p unsafe.Pointer, l C.int) *C.char {
	// XXX should use v115 stuff
	panic("unimpl")
	j, err := gob2json.ReadNode(C.GoBytes(p, l))
	if err != nil {
		log.Print(err)
		return nil
	}
	return C.CString(j)
}

//export ReadRoot
func ReadRoot(p unsafe.Pointer, l C.int) *C.char {
	j, err := gob2json.ReadRoot(C.GoBytes(p, l))
	if err != nil {
		log.Print(err)
		return nil
	}
	return C.CString(j)
}
