package main

import (
	"context"
	"fmt"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (a *app) handlePrompt(ctx context.Context, b *bot.Bot, msg *models.Message, prompt string) {
	key := fmt.Sprintf("%d:%d", msg.Chat.ID, msg.MessageThreadID)
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

	session := a.activeSession(key)
	if session == nil {
		var err error
		session, err = a.startTopicSession(ctx, key, msg.Chat.ID, msg.MessageThreadID, displayText)
		if err != nil {
			log.Printf("start codex session: %v", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          msg.Chat.ID,
				MessageThreadID: msg.MessageThreadID,
				Text:            "Dexgram hit an error while starting Codex:\n\n" + err.Error(),
			})
			return
		}
	}

	queued := a.sessionTurnCount(key) > 0
	turnID, err := startTurn(ctx, session.client, session.threadID, input, a.cfg.Codex.ApprovalPolicy, a.cfg.Codex.Sandbox)
	if err != nil {
		log.Printf("codex turn start failed: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Dexgram could not submit the message to Codex:\n\n" + err.Error(),
		})
		return
	}
	if err := a.store.ClearStagedAttachments(msg.Chat.ID, msg.MessageThreadID); err != nil {
		log.Printf("clear staged attachments: %v", err)
	}

	tgTurn := &telegramTurn{
		TurnID:          turnID,
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            displayText,
		Input:           input,
		Buffers:         map[string]string{},
		SentFiles:       map[string]bool{},
	}
	if queued {
		actionToken := a.rememberTurnAction(key, turnID)
		status, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Queued for Codex.",
			ReplyParameters: &models.ReplyParameters{
				MessageID:                msg.ID,
				ChatID:                   msg.Chat.ID,
				AllowSendingWithoutReply: true,
			},
			ReplyMarkup: turnControlMarkup(actionToken, true),
		})
		if err == nil {
			tgTurn.StatusMessageID = status.ID
		}
	} else {
		actionToken := a.rememberTurnAction(key, turnID)
		status, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Dexgram is thinking...",
			ReplyMarkup:     turnControlMarkup(actionToken, false),
		})
		if err == nil {
			tgTurn.StatusMessageID = status.ID
		}
	}
	a.addSessionTurn(key, tgTurn)
	for _, ev := range a.takePendingTurnEvents(key, turnID) {
		if a.handleTopicSessionEvent(ctx, key, session, ev) {
			return
		}
	}
	a.startTypingIndicator(key, msg.Chat.ID, msg.MessageThreadID)
}
