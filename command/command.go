package command

import (
	"fmt"
	"os"
	"os/exec"
)

type Cmd struct {
	*exec.Cmd
}

func new(name string, args ...string) *Cmd {
	return &Cmd{exec.Command(name, args...)}
}

func (cmd *Cmd) RunOrExit() (int, error) {
	code, err := cmd.Run()
	if code != 0 {
		os.Exit(code)
	}
	return code, err
}

func (cmd *Cmd) Dbg() *Cmd {
	fmt.Printf("%+v\n", cmd.Args)
	return cmd
}

func (cmd *Cmd) Run() (int, error) {
	err := cmd.Start()
	if err != nil {
		return 125, err
	}
	err = cmd.Wait()
	return cmd.ProcessState.ExitCode(), err
}

func (cmd *Cmd) RelayOutput() *Cmd {
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd
}
