package main

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const typingRefreshInterval = 45 * time.Second
const typingGlobalMinInterval = 5 * time.Second
const typingActionTimeout = 5 * time.Second
const typingRateLimitCushion = time.Second

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
	loggedError := false

	for {
		if a.sessionTurnCount(key) == 0 {
			return
		}
		if !a.reserveTypingAction(chatID, messageThreadID) {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}
		actionCtx, cancel := context.WithTimeout(ctx, typingActionTimeout)
		_, err := a.bot.SendChatAction(actionCtx, &bot.SendChatActionParams{
			ChatID:          chatID,
			MessageThreadID: messageThreadID,
			Action:          models.ChatActionTyping,
		})
		cancel()
		if err != nil && ctx.Err() == nil {
			if retryAfter, ok := logTelegramTypingPressure(chatID, messageThreadID, err); ok {
				a.suppressTypingActions(chatID, messageThreadID, retryAfter+typingRateLimitCushion)
			} else if !loggedError {
				loggedError = true
				log.Printf("send typing action: %v", err)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *app) reserveTypingAction(chatID int64, messageThreadID int) bool {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if suppressedUntil := a.typingSuppressedUntil[typingSuppressionKey(chatID, messageThreadID)]; now.Before(suppressedUntil) {
		return false
	}
	if !a.lastTypingAt.IsZero() && now.Sub(a.lastTypingAt) < typingGlobalMinInterval {
		return false
	}
	a.lastTypingAt = now
	return true
}

func (a *app) suppressTypingActions(chatID int64, messageThreadID int, delay time.Duration) {
	if delay <= 0 {
		return
	}
	until := time.Now().Add(delay)
	key := typingSuppressionKey(chatID, messageThreadID)
	a.mu.Lock()
	if a.typingSuppressedUntil == nil {
		a.typingSuppressedUntil = map[string]time.Time{}
	}
	if a.typingSuppressedUntil[key].Before(until) {
		a.typingSuppressedUntil[key] = until
	}
	a.mu.Unlock()
}

func logTelegramTypingPressure(chatID int64, messageThreadID int, err error) (time.Duration, bool) {
	var rateErr *bot.TooManyRequestsError
	if !errors.As(err, &rateErr) {
		return 0, false
	}
	retryAfter := time.Duration(rateErr.RetryAfter) * time.Second
	if retryAfter <= 0 {
		retryAfter = typingRefreshInterval
	}
	log.Printf("telegram typing pressure chat_id=%d thread_id=%d retry_after=%s reason=%q", chatID, messageThreadID, retryAfter, rateErr.Message)
	return retryAfter, true
}

func typingSuppressionKey(chatID int64, messageThreadID int) string {
	return strconv.FormatInt(chatID, 10) + ":" + strconv.Itoa(messageThreadID)
}

func (a *app) stopTypingIndicator(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if session := a.active[key]; session != nil {
		session.typing = false
	}
}
