package main

import (
	"fmt"
	"os"

	"github.com/minio/minio/cmd"
)

//GO111MODULE=on CGO_ENABLED=0 go build -tags kqueue -trimpath  foo.go
func main() {
	fmt.Printf("hi\n")
	os.Setenv("MINIO_ROOT_USER", "asdf")
	os.Setenv("MINIO_ROOT_PASSWORD", "miniostorage2")
	cmd.Main([]string{"minio", "server",
		"--address", "127.0.0.1:9001",
		"--console-address", "127.0.0.1:9002",
		"/tmp/googoo"})
}
