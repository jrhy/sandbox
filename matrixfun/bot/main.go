package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const defaultSystemPrompt = `You are a helpful Matrix room assistant.

Response policy:
- In DMs: respond to every user message.
- In group rooms: respond only when addressed (mention/reply/thread/command), unless ambient mode is enabled.
- If a message is clearly directed to another person and not you, do not respond.

Conversation policy:
- Use provided context blocks in this order of trust: current message, thread/reply context, recent room context, rolling summary, durable memory.
- Prefer thread-local context over unrelated room chatter.
- Ignore stale or irrelevant transcript items.

Elision policy:
- Greetings, small talk, acknowledgements, and dead-end tangents are low priority.
- Retain decisions, constraints, preferences, open questions, and action items.

Style:
- Be concise, accurate, and friendly.
- In group rooms, keep replies short by default.`

const defaultRestartAnnouncement = "gobot restarted and finished initial sync."

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

	ollamaModel := strings.TrimSpace(os.Getenv("OLLAMA_MODEL"))
	if ollamaModel == "" {
		ollamaModel = "mistral-small3.1"
	}
	ollamaTimeoutSeconds := parseIntEnv("OLLAMA_TIMEOUT_SECONDS", 90)
	ollamaURL := strings.TrimSpace(os.Getenv("OLLAMA_URL"))
	systemPrompt := strings.TrimSpace(os.Getenv("OLLAMA_SYSTEM_PROMPT"))
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}
	ambientMode := parseBoolEnv("MATRIX_AMBIENT_MODE")
	dmRooms := parseRoomSet(os.Getenv("MATRIX_DM_ROOMS"))
	memoryDir := strings.TrimSpace(os.Getenv("MATRIX_MEMORY_DIR"))
	statsEvery := parseIntEnv("MATRIX_DECISION_STATS_EVERY", 25)
	restartAnnouncement := strings.TrimSpace(os.Getenv("MATRIX_RESTART_ANNOUNCE_TEXT"))
	if restartAnnouncement == "" {
		restartAnnouncement = defaultRestartAnnouncement
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

	routingCfg := RoutingConfig{
		BotUserID:     login.UserID,
		CommandPrefix: "!",
		AmbientMode:   ambientMode,
	}
	llmCfg := LLMConfig{
		Model:        ollamaModel,
		SystemPrompt: systemPrompt,
		Context: ContextConfig{
			MaxMessages:     28,
			TokenBudget:     1300,
			SummaryMaxItems: 6,
		},
	}
	conversation := NewConversationStore(250)
	ollamaClient := NewHTTPOllamaClientWithTimeout(ollamaURL, time.Duration(ollamaTimeoutSeconds)*time.Second)
	memoryStore, err := NewRoomMemoryStore(memoryDir)
	if err != nil {
		return fmt.Errorf("init room memory store: %w", err)
	}
	decisionStats := newDecisionStats(statsEvery)
	var announceOnce sync.Once

	syncer.OnSync(func(syncCtx context.Context, _ *mautrix.RespSync, _ string) bool {
		announceOnce.Do(func() {
			go announceRestart(syncCtx, client, login.UserID, conversation, restartAnnouncement)
		})
		return true
	})

	syncer.OnEventType(event.EventMessage, func(handlerCtx context.Context, evt *event.Event) {
		if evt.Sender == login.UserID {
			return
		}

		roomIsDM := dmRooms[evt.RoomID]
		history := conversation.Recent(evt.RoomID, 80)
		decision, signals := ShouldRespondToEvent(handlerCtx, routingCfg, evt, roomIsDM, conversation)
		current := TranscriptFromEvent(evt, signals.MentionsBot, false)
		conversation.Add(evt.RoomID, current)
		action := "ignored"
		reacted := false
		emitDecision := func() {
			logIncomingDecision(evt, signals, decision, reacted, action)
			if summary, ok := decisionStats.Record(decision, action, reacted); ok {
				log.Printf("%s", summary)
			}
		}

		if !signals.IsText {
			emitDecision()
			return
		}

		if reply, handled := handleCommand(signals.Body, evt.RoomID, memoryStore, conversation); handled {
			if err := sendAndRecord(handlerCtx, client, login.UserID, conversation, evt.RoomID, reply); err != nil {
				log.Printf("command send failed in room %s: %v", evt.RoomID, err)
				action = "send_error"
				emitDecision()
				return
			}
			action = "command"
			reacted = true
			emitDecision()
			return
		}

		if !decision.Respond {
			emitDecision()
			return
		}

		roomMemory, err := memoryStore.Load(evt.RoomID)
		if err != nil {
			log.Printf("load room memory failed for %s: %v", evt.RoomID, err)
			roomMemory = RoomMemory{RoomID: string(evt.RoomID)}
		}
		promptCtx := BuildPromptContextForEvent(
			llmCfg.Context,
			current,
			history,
			roomMemory.RollingSummary,
			roomMemory.DurableMemory,
		)
		if _, err := client.UserTyping(handlerCtx, evt.RoomID, true, 15*time.Second); err != nil {
			log.Printf("typing start failed in room %s: %v", evt.RoomID, err)
		}
		defer func() {
			stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if _, err := client.UserTyping(stopCtx, evt.RoomID, false, 0); err != nil {
				log.Printf("typing stop failed in room %s: %v", evt.RoomID, err)
			}
		}()

		llmReply, err := CallOllama(handlerCtx, ollamaClient, llmCfg, promptCtx)
		if err != nil {
			log.Printf("ollama call failed in room %s: %v", evt.RoomID, err)
			if IsTimeoutError(err) {
				timeoutReply := fmt.Sprintf("I timed out waiting for Ollama after %ds. Please try again, or increase OLLAMA_TIMEOUT_SECONDS.", ollamaTimeoutSeconds)
				if sendErr := sendAndRecord(handlerCtx, client, login.UserID, conversation, evt.RoomID, timeoutReply); sendErr != nil {
					log.Printf("timeout notice send failed in room %s: %v", evt.RoomID, sendErr)
					action = "send_error"
					emitDecision()
					return
				}
				action = "llm_timeout"
				reacted = true
				emitDecision()
				return
			}
			action = "llm_error"
			emitDecision()
			return
		}
		if strings.TrimSpace(llmReply) == "" {
			action = "llm_empty"
			emitDecision()
			return
		}

		roomMemory.RollingSummary = promptCtx.RollingSummary
		roomMemory.DurableMemory = MergeDurableMemory(
			roomMemory.DurableMemory,
			ExtractDurableMemoryCandidates(current.Body),
			50,
		)
		if err := memoryStore.Save(evt.RoomID, roomMemory); err != nil {
			log.Printf("save room memory failed for %s: %v", evt.RoomID, err)
		}

		if err := sendAndRecord(handlerCtx, client, login.UserID, conversation, evt.RoomID, llmReply); err != nil {
			log.Printf("llm send failed in room %s: %v", evt.RoomID, err)
			action = "send_error"
			emitDecision()
			return
		}
		action = "llm_reply"
		reacted = true
		emitDecision()
	})

	log.Printf("bot running as %s", login.UserID)
	if err := client.SyncWithContext(ctx); err != nil {
		return fmt.Errorf("sync stopped: %w", err)
	}
	return nil
}

func sendAndRecord(ctx context.Context, client *mautrix.Client, botUser id.UserID, conversation *ConversationStore, roomID id.RoomID, body string) error {
	resp, err := client.SendMessageEvent(ctx, roomID, event.EventMessage, event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    body,
	})
	if err != nil {
		return fmt.Errorf("send message to %s: %w", roomID, err)
	}
	conversation.Add(roomID, TranscriptMessage{
		EventID:   string(resp.EventID),
		RoomID:    string(roomID),
		Sender:    string(botUser),
		Body:      body,
		IsBot:     true,
		Timestamp: time.Now().UTC(),
	})
	return nil
}

func parseBoolEnv(key string) bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return val == "1" || val == "true" || val == "yes" || val == "on"
}

func parseRoomSet(raw string) map[id.RoomID]bool {
	out := make(map[id.RoomID]bool)
	for _, part := range strings.Split(raw, ",") {
		room := strings.TrimSpace(part)
		if room == "" {
			continue
		}
		out[id.RoomID(room)] = true
	}
	return out
}

func parseIntEnv(key string, fallback int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func logIncomingDecision(evt *event.Event, signals MessageSignals, decision RoutingDecision, reacted bool, action string) {
	log.Printf(
		"incoming room=%s event=%s sender=%s text=%t mentions_bot=%t decision=%s reacted=%t action=%s body=%q",
		evt.RoomID,
		evt.ID,
		evt.Sender,
		signals.IsText,
		signals.MentionsBot,
		decision.Reason,
		reacted,
		action,
		trimForLog(signals.Body, 180),
	)
}

func trimForLog(body string, maxLen int) string {
	body = strings.TrimSpace(body)
	if maxLen <= 0 || len(body) <= maxLen {
		return body
	}
	return body[:maxLen] + "..."
}

type decisionStats struct {
	mu      sync.Mutex
	every   int
	total   int
	reacted int
	reasons map[string]int
	actions map[string]int
}

func newDecisionStats(every int) *decisionStats {
	if every <= 0 {
		every = 25
	}
	return &decisionStats{
		every:   every,
		reasons: make(map[string]int),
		actions: make(map[string]int),
	}
}

func (d *decisionStats) Record(decision RoutingDecision, action string, reacted bool) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.total++
	if reacted {
		d.reacted++
	}
	reason := strings.TrimSpace(decision.Reason)
	if reason == "" {
		reason = "unknown"
	}
	d.reasons[reason]++
	action = strings.TrimSpace(action)
	if action == "" {
		action = "unknown"
	}
	d.actions[action]++

	if d.total%d.every != 0 {
		return "", false
	}

	return fmt.Sprintf(
		"decision-stats window=%d total=%d reacted=%d reaction_rate=%.2f%% reasons=%s actions=%s",
		d.every,
		d.total,
		d.reacted,
		100*float64(d.reacted)/float64(d.total),
		formatCounts(d.reasons),
		formatCounts(d.actions),
	), true
}

func formatCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func announceRestart(ctx context.Context, client *mautrix.Client, botUser id.UserID, conversation *ConversationStore, message string) {
	if strings.TrimSpace(message) == "" {
		return
	}
	joined, err := client.JoinedRooms(ctx)
	if err != nil {
		log.Printf("restart announcement skipped: joined rooms lookup failed: %v", err)
		return
	}
	sent := 0
	failed := 0
	for _, roomID := range joined.JoinedRooms {
		if err := sendAndRecord(ctx, client, botUser, conversation, roomID, message); err != nil {
			failed++
			log.Printf("restart announcement send failed in room %s: %v", roomID, err)
			continue
		}
		sent++
	}
	log.Printf("restart announcement sent=%d failed=%d room(s)", sent, failed)
}
