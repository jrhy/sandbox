package command_test

import (
	"os"

	"github.com/jrhy/sandbox/command"
)

func ExampleNew() {
	cmd := command.New("go", "build", "-o", "server-linux",
		"./cmd/server")
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH=amd64",
	)
	cmd.Dbg().RelayOutput().RunOrExit()

	code, _ := command.New("cmp", ".server-linux-deployed",
		"server-linux").
		Dbg().RelayOutput().Run()
	if code == 0 {
		return
	}

	command.New(
		"ssh", "root@example.com",
		"pkill server || true").
		Dbg().RelayOutput().RunOrExit()
	command.New(
		"scp", "-C", "server-linux", "root@example.com:server").
		Dbg().RelayOutput().RunOrExit()
	command.New(
		"ssh", "root@example.com",
		"./server < /dev/null >> server.log 2>&1 &").
		Dbg().RelayOutput().RunOrExit()
	command.New(
		"cp", "./server-linux", ".server-linux-deployed").
		Dbg().RelayOutput().RunOrExit()
}
