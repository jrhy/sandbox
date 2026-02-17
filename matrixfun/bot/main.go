package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

func mustEnv(key string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		log.Fatalf("missing env var %s", key)
	}
	return val
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "bot stopped: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	homeserver := mustEnv("MATRIX_HOMESERVER")
	user := mustEnv("MATRIX_USER")
	pass := mustEnv("MATRIX_PASS")
	room := strings.TrimSpace(os.Getenv("MATRIX_ROOM"))
	stateFile := strings.TrimSpace(os.Getenv("MATRIX_STATE_FILE"))
	if stateFile == "" {
		stateFile = ".matrix-bot-state.json"
	}

	client, err := mautrix.NewClient(homeserver, "", "")
	if err != nil {
		return fmt.Errorf("new client: %w", err)
	}

	login, err := client.Login(ctx, &mautrix.ReqLogin{
		Type: "m.login.password",
		Identifier: mautrix.UserIdentifier{
			Type: mautrix.IdentifierTypeUser,
			User: user,
		},
		Password: pass,
	})
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	client.SetCredentials(login.UserID, login.AccessToken)
	store, err := newFileSyncStore(stateFile)
	if err != nil {
		return fmt.Errorf("init sync store: %w", err)
	}
	client.Store = store

	if room != "" {
		if _, err := client.JoinRoom(ctx, room, nil); err != nil {
			log.Printf("join room %q failed: %v", room, err)
		} else {
			log.Printf("joined room %s", room)
		}
	}

	syncer, ok := client.Syncer.(*mautrix.DefaultSyncer)
	if !ok {
		return fmt.Errorf("unexpected syncer type %T", client.Syncer)
	}

	syncer.OnEventType(event.EventMessage, func(_ context.Context, evt *event.Event) {
		if evt.Sender == login.UserID {
			return
		}
		msg := evt.Content.AsMessage()
		if msg == nil || msg.MsgType != event.MsgText {
			return
		}
		body := strings.TrimSpace(msg.Body)
		if body == "" {
			return
		}

		var reply string
		switch {
		case body == "!ping":
			reply = "pong"
		case strings.HasPrefix(body, "!echo "):
			reply = strings.TrimSpace(strings.TrimPrefix(body, "!echo "))
		case body == "!help":
			reply = "Commands: !ping, !echo <text>, !help"
		}

		if reply == "" {
			return
		}

		_, _ = client.SendMessageEvent(ctx, evt.RoomID, event.EventMessage, event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    reply,
		})
	})

	log.Printf("bot running as %s", login.UserID)
	if err := client.SyncWithContext(ctx); err != nil {
		return fmt.Errorf("sync stopped: %w", err)
	}
	return nil
}
