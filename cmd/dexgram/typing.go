package main

import (
	"context"
	"log"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const typingRefreshInterval = 4 * time.Second

func (a *app) startTypingIndicator(key string, chatID int64, messageThreadID int) {
	a.mu.Lock()
	session := a.active[key]
	if session == nil || session.typing {
		a.mu.Unlock()
		return
	}
	session.typing = true
	ctx := session.ctx
	a.mu.Unlock()

	go a.keepTyping(ctx, key, chatID, messageThreadID)
}

func (a *app) keepTyping(ctx context.Context, key string, chatID int64, messageThreadID int) {
	defer a.stopTypingIndicator(key)

	ticker := time.NewTicker(typingRefreshInterval)
	defer ticker.Stop()

	for {
		if a.sessionTurnCount(key) == 0 {
			return
		}
		if _, err := a.bot.SendChatAction(ctx, &bot.SendChatActionParams{
			ChatID:          chatID,
			MessageThreadID: messageThreadID,
			Action:          models.ChatActionTyping,
		}); err != nil && ctx.Err() == nil {
			log.Printf("send typing action: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *app) stopTypingIndicator(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if session := a.active[key]; session != nil {
		session.typing = false
	}
}
