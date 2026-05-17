package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (a *app) handlePrompt(ctx context.Context, b *bot.Bot, msg *models.Message, prompt string) {
	a.handlePromptWithMode(ctx, b, msg, prompt, "")
}

func (a *app) handlePlanPrompt(ctx context.Context, b *bot.Bot, msg *models.Message, prompt string) {
	a.handlePromptWithMode(ctx, b, msg, prompt, "plan")
}

func (a *app) handlePromptWithMode(ctx context.Context, b *bot.Bot, msg *models.Message, prompt, collaborationMode string) {
	input, displayText, err := a.buildTurnInput(ctx, b, msg, prompt)
	if err != nil {
		log.Printf("build turn input failed: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Dexgram could not read the Telegram attachment:\n\n" + err.Error(),
		})
		return
	}

	if collaborationMode == "" && a.shouldAskFreshTopicChoice(msg.Chat.ID, msg.MessageThreadID) {
		a.askFreshTopicChoice(ctx, b, msg, input, displayText)
		return
	}

	a.submitBuiltPrompt(ctx, b, msg.Chat.ID, msg.MessageThreadID, msg.ID, input, displayText, collaborationMode)
}

func (a *app) submitBuiltPrompt(ctx context.Context, b *bot.Bot, chatID int64, messageThreadID int, replyMessageID int, input []map[string]any, displayText, collaborationMode string) {
	key := fmt.Sprintf("%d:%d", chatID, messageThreadID)
	session := a.activeSession(key)
	if session == nil {
		var err error
		session, err = a.startTopicSession(ctx, key, chatID, messageThreadID, displayText)
		if err != nil {
			log.Printf("start codex session: %v", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chatID,
				MessageThreadID: messageThreadID,
				Text:            "Dexgram hit an error while starting Codex:\n\n" + err.Error(),
			})
			return
		}
	}

	queued := a.sessionTurnCount(key) > 0
	var turnID string
	if !queued {
		var err error
		opts, optErr := a.turnOptions(ctx, session.client, collaborationMode)
		if optErr != nil {
			log.Printf("codex turn options failed: %v", optErr)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chatID,
				MessageThreadID: messageThreadID,
				Text:            "Dexgram could not prepare the message for Codex:\n\n" + optErr.Error(),
			})
			return
		}
		a.syncTelegramPromptTranscript(chatID, messageThreadID, replyMessageID, session.threadID, displayText)
		a.beginSessionStartingTurn(key, session)
		turnID, err = startTurn(ctx, session.client, session.threadID, telegramPromptInput(input), opts)
		if err != nil {
			a.endSessionStartingTurn(key, session)
			log.Printf("codex turn start failed: %v", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chatID,
				MessageThreadID: messageThreadID,
				Text:            "Dexgram could not submit the message to Codex:\n\n" + err.Error(),
			})
			return
		}
		a.markTelegramTurn(session.threadID, turnID, chatID, messageThreadID, replyMessageID)
	} else {
		turnID = a.nextQueuedTurnID()
	}
	if err := a.store.ClearStagedAttachments(chatID, messageThreadID); err != nil {
		log.Printf("clear staged attachments: %v", err)
	}

	tgTurn := &telegramTurn{
		TurnID:            turnID,
		Queued:            queued,
		ChatID:            chatID,
		MessageThreadID:   messageThreadID,
		SourceMessageID:   replyMessageID,
		Text:              displayText,
		Input:             input,
		CollaborationMode: normalizeCollaborationMode(collaborationMode),
		Buffers:           map[string]string{},
		SentFiles:         map[string]bool{},
	}
	if queued {
		actionToken := a.rememberTurnAction(key, turnID)
		status, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: messageThreadID,
			Text:            "Queued for Codex.",
			ReplyParameters: &models.ReplyParameters{
				MessageID:                replyMessageID,
				ChatID:                   chatID,
				AllowSendingWithoutReply: true,
			},
			ReplyMarkup:         turnControlMarkup(actionToken, true),
			DisableNotification: true,
		})
		if err == nil {
			tgTurn.StatusMessageID = status.ID
		}
	} else {
		actionToken := a.rememberTurnAction(key, turnID)
		status, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:              chatID,
			MessageThreadID:     messageThreadID,
			Text:                "Dexgram is thinking...",
			ReplyMarkup:         turnControlMarkup(actionToken, false),
			DisableNotification: true,
		})
		if err == nil {
			tgTurn.StatusMessageID = status.ID
		}
	}
	a.addSessionTurn(key, tgTurn)
	for _, ev := range a.takePendingTurnEvents(key, turnID) {
		if a.handleTopicSessionEvent(ctx, key, session, ev) {
			a.endSessionStartingTurn(key, session)
			return
		}
	}
	if !queued {
		a.endSessionStartingTurn(key, session)
	}
	a.startTypingIndicator(key, chatID, messageThreadID)
}

func (a *app) markTelegramTurn(codexThreadID, turnID string, chatID int64, messageThreadID, messageID int) {
	if err := a.store.SaveTelegramTurn(codexThreadID, turnID, chatID, messageThreadID, messageID); err != nil {
		log.Printf("save telegram turn marker chat_id=%d thread_id=%d turn_id=%s: %v", chatID, messageThreadID, turnID, err)
	}
}

func (a *app) shouldAskFreshTopicChoice(chatID int64, messageThreadID int) bool {
	conv, ok, err := a.store.Get(chatID, messageThreadID)
	if err != nil {
		log.Printf("read conversation before fresh topic choice: %v", err)
		return false
	}
	if !ok {
		return true
	}
	return conv.CodexThreadID == "" &&
		strings.TrimSpace(conv.ProjectName) == "" &&
		strings.TrimSpace(conv.CWD) == ""
}

func pendingFreshTopicCommandName(pending *pendingFreshTopic) (string, bool) {
	if pending == nil {
		return "", false
	}
	name, _, ok := parseTelegramCommand(pending.displayText)
	return name, ok
}

func (a *app) askFreshTopicChoice(ctx context.Context, b *bot.Bot, msg *models.Message, input []map[string]any, displayText string) {
	token := a.rememberFreshTopic(&pendingFreshTopic{
		chatID:          msg.Chat.ID,
		messageThreadID: msg.MessageThreadID,
		replyMessageID:  msg.ID,
		input:           input,
		displayText:     displayText,
		createdAt:       time.Now(),
	})
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            "How should Dexgram use this message?",
		ReplyParameters: &models.ReplyParameters{
			MessageID:                msg.ID,
			ChatID:                   msg.Chat.ID,
			AllowSendingWithoutReply: true,
		},
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Resume session", CallbackData: "fresh:" + token + ":sessions"}},
			{{Text: "Start new chat", CallbackData: "fresh:" + token + ":new"}},
			{{Text: "Set project first", CallbackData: "fresh:" + token + ":project"}},
		}},
		DisableNotification: true,
	})
}

func (a *app) rememberFreshTopic(pending *pendingFreshTopic) string {
	token := strconv.FormatInt(a.freshSeq.Add(1), 36)
	a.mu.Lock()
	if a.freshTopics == nil {
		a.freshTopics = map[string]*pendingFreshTopic{}
	}
	a.freshTopics[token] = pending
	a.mu.Unlock()
	return token
}

func (a *app) takeFreshTopic(token string) (*pendingFreshTopic, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	pending := a.freshTopics[token]
	if pending == nil {
		return nil, false
	}
	delete(a.freshTopics, token)
	return pending, true
}

func (a *app) freshTopic(token string) (*pendingFreshTopic, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	pending := a.freshTopics[token]
	return pending, pending != nil
}
