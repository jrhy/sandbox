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
- In group rooms, keep replies short by default.

Mutation safety:
- Do not claim state changes (memory/policy/task updates) unless a real command path performed them.
- If the user asks for a mutation in plain language, ask them to use explicit commands.
- For memory edits, prefer commands like: !memory, !memory forget <text>, !memory clear.
- For policy edits, prefer commands like: !policy show, !policy set ..., !policy edit ..., !policy reset.`

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
	policyMirrorDir := strings.TrimSpace(os.Getenv("MATRIX_POLICY_MIRROR_DIR"))
	statsEvery := parseIntEnv("MATRIX_DECISION_STATS_EVERY", 25)
	restartAnnouncement := strings.TrimSpace(os.Getenv("MATRIX_RESTART_ANNOUNCE_TEXT"))
	if restartAnnouncement == "" {
		restartAnnouncement = defaultRestartAnnouncement
	}
	trustedEditors := parseUserSet(os.Getenv("BOT_POLICY_TRUSTED_EDITORS"))
	chatAPIListen := strings.TrimSpace(os.Getenv("BOT_API_LISTEN"))
	chatAPIToken := strings.TrimSpace(os.Getenv("BOT_API_TOKEN"))

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
	memoryStore, err := NewRoomMemoryStoreWithMirror(memoryDir, policyMirrorDir)
	if err != nil {
		return fmt.Errorf("init room memory store: %w", err)
	}
	decisionStats := newDecisionStats(statsEvery)
	botRuntime := &BotRuntime{
		LLMConfig:            llmCfg,
		OllamaClient:         ollamaClient,
		Conversation:         conversation,
		MemoryStore:          memoryStore,
		OllamaTimeoutSeconds: ollamaTimeoutSeconds,
		TrustedPolicyEditors: trustedEditors,
	}

	if err := startChatAPIServer(ctx, chatAPIListen, botRuntime, chatAPIToken); err != nil {
		return fmt.Errorf("start chat api server: %w", err)
	}

	loopCfg := matrixLoopConfig{
		Homeserver:          homeserver,
		User:                user,
		Pass:                pass,
		Room:                room,
		StateFile:           stateFile,
		DMRooms:             dmRooms,
		AmbientMode:         ambientMode,
		RestartAnnouncement: restartAnnouncement,
		Runtime:             botRuntime,
		DecisionStats:       decisionStats,
	}

	go matrixLoop(ctx, loopCfg)

	<-ctx.Done()
	return nil
}

type matrixLoopConfig struct {
	Homeserver          string
	User                string
	Pass                string
	Room                string
	StateFile           string
	DMRooms             map[id.RoomID]bool
	AmbientMode         bool
	RestartAnnouncement string
	Runtime             *BotRuntime
	DecisionStats       *decisionStats
}

func matrixLoop(ctx context.Context, cfg matrixLoopConfig) {
	delay := 1 * time.Second
	maxDelay := 5 * time.Minute
	var announceOnce sync.Once

	for {
		if ctx.Err() != nil {
			return
		}

		start := time.Now()
		err := runMatrixSession(ctx, cfg, &announceOnce)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("matrix session ended with error: %v", err)
		} else {
			log.Printf("matrix session ended")
		}

		if time.Since(start) >= 60*time.Second {
			delay = 1 * time.Second
		}

		log.Printf("matrix reconnect in %s", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func runMatrixSession(ctx context.Context, cfg matrixLoopConfig, announceOnce *sync.Once) error {
	client, err := mautrix.NewClient(cfg.Homeserver, "", "")
	if err != nil {
		return fmt.Errorf("new client: %w", err)
	}

	login, err := client.Login(ctx, &mautrix.ReqLogin{
		Type: "m.login.password",
		Identifier: mautrix.UserIdentifier{
			Type: mautrix.IdentifierTypeUser,
			User: cfg.User,
		},
		Password: cfg.Pass,
	})
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	client.SetCredentials(login.UserID, login.AccessToken)

	store, err := newFileSyncStore(cfg.StateFile)
	if err != nil {
		return fmt.Errorf("init sync store: %w", err)
	}
	client.Store = store

	if cfg.Room != "" {
		if _, err := client.JoinRoom(ctx, cfg.Room, nil); err != nil {
			log.Printf("join room %q failed: %v", cfg.Room, err)
		} else {
			log.Printf("joined room %s", cfg.Room)
		}
	}

	syncer, ok := client.Syncer.(*mautrix.DefaultSyncer)
	if !ok {
		return fmt.Errorf("unexpected syncer type %T", client.Syncer)
	}

	syncer.OnSync(func(syncCtx context.Context, _ *mautrix.RespSync, _ string) bool {
		announceOnce.Do(func() {
			go announceRestart(syncCtx, client, login.UserID, cfg.Runtime.Conversation, cfg.RestartAnnouncement)
		})
		return true
	})

	routingCfg := RoutingConfig{
		BotUserID:     login.UserID,
		CommandPrefix: "!",
		AmbientMode:   cfg.AmbientMode,
	}

	syncer.OnEventType(event.EventMessage, func(handlerCtx context.Context, evt *event.Event) {
		if evt.Sender == login.UserID {
			return
		}

		roomIsDM := cfg.DMRooms[evt.RoomID]
		history := cfg.Runtime.Conversation.Recent(evt.RoomID, 80)
		decision, signals := ShouldRespondToEvent(handlerCtx, routingCfg, evt, roomIsDM, cfg.Runtime.Conversation)
		current := TranscriptFromEvent(evt, signals.MentionsBot, false)
		cfg.Runtime.Conversation.Add(evt.RoomID, current)
		action := "ignored"
		reacted := false
		emitDecision := func() {
			logIncomingDecision(evt, signals, decision, reacted, action)
			if summary, ok := cfg.DecisionStats.Record(decision, action, reacted); ok {
				log.Printf("%s", summary)
			}
		}

		if !signals.IsText {
			emitDecision()
			return
		}

		if reply, handled, err := handlePolicyCommand(handlerCtx, evt.Sender, evt.RoomID, signals.Body, cfg.Runtime.MemoryStore, cfg.Runtime.LLMConfig, cfg.Runtime.OllamaClient, cfg.Runtime.TrustedPolicyEditors); handled {
			if err != nil {
				log.Printf("policy command failed in room %s: %v", evt.RoomID, err)
				action = "policy_error"
				emitDecision()
				return
			}
			if err := sendAndRecord(handlerCtx, client, login.UserID, cfg.Runtime.Conversation, evt.RoomID, reply); err != nil {
				log.Printf("policy send failed in room %s: %v", evt.RoomID, err)
				action = "send_error"
				emitDecision()
				return
			}
			action = "policy_command"
			reacted = true
			emitDecision()
			return
		}

		if reply, handled := handleCommand(signals.Body, evt.RoomID, cfg.Runtime.MemoryStore, cfg.Runtime.Conversation); handled {
			if err := sendAndRecord(handlerCtx, client, login.UserID, cfg.Runtime.Conversation, evt.RoomID, reply); err != nil {
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

		roomMemory, err := cfg.Runtime.MemoryStore.Load(evt.RoomID)
		if err != nil {
			log.Printf("load room memory failed for %s: %v", evt.RoomID, err)
			roomMemory = RoomMemory{RoomID: string(evt.RoomID)}
		}
		activeLLMCfg := cfg.Runtime.LLMConfig
		activeLLMCfg.SystemPrompt = composeSystemPrompt(cfg.Runtime.LLMConfig.SystemPrompt, roomMemory.Policy)
		activeLLMCfg.Context = applyPolicyToContextConfig(cfg.Runtime.LLMConfig.Context, roomMemory.Policy)
		promptCtx := BuildPromptContextForEvent(
			activeLLMCfg.Context,
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

		llmReply, err := CallOllama(handlerCtx, cfg.Runtime.OllamaClient, activeLLMCfg, promptCtx)
		if err != nil {
			log.Printf("ollama call failed in room %s: %v", evt.RoomID, err)
			if IsTimeoutError(err) {
				timeoutReply := fmt.Sprintf("I timed out waiting for Ollama after %ds. Please try again, or increase OLLAMA_TIMEOUT_SECONDS.", cfg.Runtime.OllamaTimeoutSeconds)
				if sendErr := sendAndRecord(handlerCtx, client, login.UserID, cfg.Runtime.Conversation, evt.RoomID, timeoutReply); sendErr != nil {
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
		if err := cfg.Runtime.MemoryStore.Save(evt.RoomID, roomMemory); err != nil {
			log.Printf("save room memory failed for %s: %v", evt.RoomID, err)
		}

		if err := sendAndRecord(handlerCtx, client, login.UserID, cfg.Runtime.Conversation, evt.RoomID, llmReply); err != nil {
			log.Printf("llm send failed in room %s: %v", evt.RoomID, err)
			action = "send_error"
			emitDecision()
			return
		}
		action = "llm_reply"
		reacted = true
		emitDecision()
	})

	log.Printf("matrix connected as %s", login.UserID)
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

func parseUserSet(raw string) map[id.UserID]bool {
	out := make(map[id.UserID]bool)
	for _, part := range strings.Split(raw, ",") {
		user := strings.TrimSpace(part)
		if user == "" {
			continue
		}
		out[id.UserID(user)] = true
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
