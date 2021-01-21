package main

// int rfg_int();
// #cgo LDFLAGS: -Loutput/ -lrfg
import "C"
import "fmt"

func main() {
	fmt.Println("The next line is a number from rfg_int.rs:")
	fmt.Println(C.rfg_int())
}
