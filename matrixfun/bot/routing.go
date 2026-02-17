package main

import (
	"context"
	"strings"
	"unicode"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

const defaultCommandPrefix = "!"

type RoutingConfig struct {
	BotUserID     id.UserID
	CommandPrefix string
	AmbientMode   bool
}

type RoutingDecision struct {
	Respond bool
	Reason  string
}

type MessageSignals struct {
	IsText              bool
	IsDM                bool
	Body                string
	Sender              id.UserID
	StartsWithCommand   bool
	MentionsBot         bool
	MentionsOtherUsers  bool
	ReplyToEventID      id.EventID
	ThreadParentEventID id.EventID
	ReplyTargetsBot     bool
	ThreadParentIsBot   bool
	DirectedToOtherUser bool
}

type EventSenderLookup interface {
	SenderForEvent(ctx context.Context, roomID id.RoomID, eventID id.EventID) (id.UserID, bool)
}

func EvaluateRouting(ctx context.Context, cfg RoutingConfig, evt *event.Event, roomIsDM bool, lookup EventSenderLookup) (RoutingDecision, MessageSignals) {
	signals := ExtractSignals(cfg, evt, roomIsDM)
	if lookup != nil {
		if signals.ReplyToEventID != "" {
			sender, ok := lookup.SenderForEvent(ctx, evt.RoomID, signals.ReplyToEventID)
			signals.ReplyTargetsBot = ok && sender == cfg.BotUserID
		}
		if signals.ThreadParentEventID != "" {
			sender, ok := lookup.SenderForEvent(ctx, evt.RoomID, signals.ThreadParentEventID)
			signals.ThreadParentIsBot = ok && sender == cfg.BotUserID
		}
	}
	return ShouldRespond(cfg, signals), signals
}

func ExtractSignals(cfg RoutingConfig, evt *event.Event, roomIsDM bool) MessageSignals {
	signals := MessageSignals{IsDM: roomIsDM}
	if evt == nil {
		return signals
	}
	signals.Sender = evt.Sender

	msg := evt.Content.AsMessage()
	if msg == nil || msg.MsgType != event.MsgText {
		return signals
	}

	body := strings.TrimSpace(msg.Body)
	if body == "" {
		return signals
	}

	signals.IsText = true
	signals.Body = body
	signals.StartsWithCommand = hasCommandPrefix(cfg.CommandPrefix, body)
	signals.ReplyToEventID = msg.GetReplyTo()
	if msg.RelatesTo != nil {
		signals.ThreadParentEventID = msg.RelatesTo.GetThreadParent()
	}

	if msg.Mentions != nil {
		for _, userID := range msg.Mentions.UserIDs {
			if userID == cfg.BotUserID {
				signals.MentionsBot = true
				continue
			}
			signals.MentionsOtherUsers = true
		}
	}

	signals.DirectedToOtherUser = signals.MentionsOtherUsers && !signals.MentionsBot
	if !signals.DirectedToOtherUser {
		target, ok := leadingBodyTarget(body)
		if ok {
			if looksLikeBotMention(target, cfg.BotUserID) {
				signals.MentionsBot = true
			} else {
				signals.DirectedToOtherUser = true
			}
		}
	}

	return signals
}

func ShouldRespond(cfg RoutingConfig, signals MessageSignals) RoutingDecision {
	if !signals.IsText {
		return RoutingDecision{Reason: "not_text"}
	}

	if signals.IsDM {
		return RoutingDecision{Respond: true, Reason: "dm"}
	}

	if signals.StartsWithCommand {
		return RoutingDecision{Respond: true, Reason: "command"}
	}

	if signals.MentionsBot {
		return RoutingDecision{Respond: true, Reason: "mention"}
	}

	if signals.ReplyTargetsBot {
		return RoutingDecision{Respond: true, Reason: "reply_to_bot"}
	}

	if signals.ThreadParentIsBot {
		return RoutingDecision{Respond: true, Reason: "bot_thread"}
	}

	if signals.DirectedToOtherUser {
		return RoutingDecision{Reason: "directed_to_other_user"}
	}

	if cfg.AmbientMode {
		return RoutingDecision{Respond: true, Reason: "ambient"}
	}

	return RoutingDecision{Reason: "not_addressed"}
}

func hasCommandPrefix(prefix, body string) bool {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = defaultCommandPrefix
	}
	return strings.HasPrefix(strings.TrimSpace(body), prefix)
}

func leadingBodyTarget(body string) (string, bool) {
	body = strings.TrimSpace(body)
	if body == "" || !strings.HasPrefix(body, "@") {
		return "", false
	}

	var b strings.Builder
	for i, r := range body {
		if i == 0 {
			b.WriteRune(r)
			continue
		}
		if unicode.IsSpace(r) || r == ':' || r == ',' {
			break
		}
		b.WriteRune(r)
	}
	target := b.String()
	if len(target) <= 1 {
		return "", false
	}
	return target, true
}

func looksLikeBotMention(target string, botUserID id.UserID) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	bot := strings.ToLower(strings.TrimSpace(string(botUserID)))
	if bot != "" && target == bot {
		return true
	}
	local := strings.TrimPrefix(bot, "@")
	if i := strings.Index(local, ":"); i >= 0 {
		local = local[:i]
	}
	local = strings.TrimSpace(local)
	if local == "" {
		return false
	}
	return target == "@"+local
}
