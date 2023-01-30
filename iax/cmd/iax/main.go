package main

import (
	"fmt"
	"os"

	"github.com/jrhy/sandbox/iax"
)

func main() {
	endpoint := os.Getenv("IAX_ENDPOINT")
	if endpoint == "" {
		endpoint = "vancouver2.voip.ms:4569"
	}
	u := iax.User{Username: os.Getenv("IAX_USER"), Password: os.Getenv("IAX_PASSWORD")}
	if u.Username == "" || u.Password == "" {
		fmt.Printf("set IAX_USER, IAX_PASSWORD, IAX_ENDPOINT\n")
		os.Exit(1)
	}
	p := iax.NewPeer(endpoint, u)
	p.Debug = func(format string, args ...interface{}) { fmt.Printf(format, args...) }
	err := p.RegisterAndServe()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n")
}
