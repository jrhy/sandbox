package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jessevdk/go-flags"
	"github.com/matrix-org/gomatrix"
)

type Matrix struct {
	Password string `short:"p"`
	RoomID   string `short:"r"`
	Server   string `short:"s" long:"server"`
	Token    string `short:"t"`
	Username string `short:"u"`
}

func init() {
	funcs["matrix"] = subcommand{
		`[-u <username> -p <password>] [-t <token>] [-s/--server="https://matrix.org"] [-r <room ID>] <message...>`,
		"send a matrix message; login with password first (uses ~/.config/matrixtoken)",
		func(a []string) int {
			o := Matrix{Server: "https://matrix.org"}
			p := flags.NewParser(&o, 0)
			other, err := p.ParseArgs(a)
			if err != nil {
				if strings.Contains(err.Error(), "unknown flag") {
					fmt.Printf("%s\n", err.Error())
					return exitSubcommandUsage
				}
				die(fmt.Sprintf("parse: %v", err))
			}
			if err := o.Send(strings.Join(other, " ")); err != nil {
				fmt.Printf("%v\n", err)
				return 1
			}
			return 0
		},
	}
}

func (m *Matrix) Send(message string) error {

	if message == "" {
		return errors.New("empty message")
	}
	c, err := gomatrix.NewClient(m.Server, "", m.Token)
	if err != nil {
		return err
	}
	if m.RoomID == "" {
		return errors.New("no room ID")
	}
	if m.Token == "" {
		filename := fmt.Sprintf("%s/.config/matrixtoken", os.Getenv("HOME"))
		if m.Username == "" {
			token, err := os.ReadFile(filename)
			if err != nil {
				return fmt.Errorf("no username or token; %s: %w", filename, err)
			}
			m.Token = strings.TrimSpace(string(token))
			c.SetCredentials("", m.Token)
		} else {
			resp, err := c.Login(&gomatrix.ReqLogin{
				Type:     "m.login.password",
				User:     m.Username,
				Password: m.Password,
			})
			if err != nil {
				return fmt.Errorf("login: %w", err)
			}
			fmt.Printf("login successful: %+v\n", resp)
			err = os.WriteFile(filename, []byte(resp.AccessToken), 0400)
			if err != nil {
				return fmt.Errorf("%s: %w", filename, err)
			}
			fmt.Printf("recorded token at %s\n", filename)
			m.Token = resp.AccessToken
			c.SetCredentials("", m.Token)
		}
	}
	resp, err := c.SendText(m.RoomID, message)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	fmt.Printf("send successful: %+v\n", resp)
	return nil
}
