package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type LLMConfig struct {
	Model        string
	SystemPrompt string
	Context      ContextConfig
}

type ConversationStore struct {
	mu         sync.Mutex
	byRoom     map[id.RoomID][]TranscriptMessage
	byEvent    map[id.EventID]TranscriptMessage
	maxPerRoom int
}

func NewConversationStore(maxPerRoom int) *ConversationStore {
	if maxPerRoom <= 0 {
		maxPerRoom = 200
	}
	return &ConversationStore{
		byRoom:     make(map[id.RoomID][]TranscriptMessage),
		byEvent:    make(map[id.EventID]TranscriptMessage),
		maxPerRoom: maxPerRoom,
	}
}

func (s *ConversationStore) Add(roomID id.RoomID, msg TranscriptMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	roomMessages := append(s.byRoom[roomID], msg)
	if len(roomMessages) > s.maxPerRoom {
		dropped := roomMessages[:len(roomMessages)-s.maxPerRoom]
		for _, old := range dropped {
			delete(s.byEvent, id.EventID(old.EventID))
		}
		roomMessages = roomMessages[len(roomMessages)-s.maxPerRoom:]
	}
	s.byRoom[roomID] = roomMessages
	if msg.EventID != "" {
		s.byEvent[id.EventID(msg.EventID)] = msg
	}
}

func (s *ConversationStore) Recent(roomID id.RoomID, limit int) []TranscriptMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	roomMessages := s.byRoom[roomID]
	if limit <= 0 || len(roomMessages) <= limit {
		return append([]TranscriptMessage(nil), roomMessages...)
	}
	return append([]TranscriptMessage(nil), roomMessages[len(roomMessages)-limit:]...)
}

func (s *ConversationStore) SenderForEvent(_ context.Context, _ id.RoomID, eventID id.EventID) (id.UserID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, ok := s.byEvent[eventID]
	if !ok || msg.Sender == "" {
		return "", false
	}
	return id.UserID(msg.Sender), true
}

func (s *ConversationStore) ClearRoom(roomID id.RoomID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, msg := range s.byRoom[roomID] {
		if msg.EventID != "" {
			delete(s.byEvent, id.EventID(msg.EventID))
		}
	}
	delete(s.byRoom, roomID)
}

func ShouldRespondToEvent(ctx context.Context, cfg RoutingConfig, evt *event.Event, roomIsDM bool, lookup EventSenderLookup) (RoutingDecision, MessageSignals) {
	return EvaluateRouting(ctx, cfg, evt, roomIsDM, lookup)
}

func BuildPromptContextForEvent(ctxCfg ContextConfig, current TranscriptMessage, history []TranscriptMessage, rollingSummary string, durableMemory []string) PromptContext {
	return BuildPromptContext(ctxCfg, current, history, rollingSummary, durableMemory)
}

func PruneAndSummarize(ctxCfg ContextConfig, current TranscriptMessage, history []TranscriptMessage, rollingSummary string) (included []TranscriptMessage, summary string) {
	promptCtx := BuildPromptContext(ctxCfg, current, history, rollingSummary, nil)
	return promptCtx.Included, promptCtx.RollingSummary
}

func CallOllama(ctx context.Context, client OllamaClient, cfg LLMConfig, promptCtx PromptContext) (string, error) {
	if client == nil {
		return "", fmt.Errorf("nil ollama client")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return "", fmt.Errorf("missing model")
	}

	req := BuildChatRequest(cfg.Model, cfg.SystemPrompt, promptCtx)
	resp, err := client.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Message.Content), nil
}

func TranscriptFromEvent(evt *event.Event, mentionsBot bool, isBot bool) TranscriptMessage {
	msg := evt.Content.AsMessage()
	body := ""
	replyTo := ""
	threadRoot := ""
	if msg != nil {
		body = strings.TrimSpace(msg.Body)
		replyTo = string(msg.GetReplyTo())
		if msg.RelatesTo != nil {
			threadRoot = string(msg.RelatesTo.GetThreadParent())
		}
	}
	return TranscriptMessage{
		EventID:     string(evt.ID),
		RoomID:      string(evt.RoomID),
		Sender:      string(evt.Sender),
		Body:        body,
		Timestamp:   time.UnixMilli(evt.Timestamp),
		ReplyTo:     replyTo,
		ThreadRoot:  threadRoot,
		MentionsBot: mentionsBot,
		IsBot:       isBot,
	}
}
