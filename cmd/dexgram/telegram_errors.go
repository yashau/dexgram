package main

import (
	"errors"
	"strings"

	"dexgram/internal/state"

	"github.com/go-telegram/bot"
)

var errTelegramTopicGone = errors.New("telegram topic gone")

func isTelegramTopicGoneError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return errors.Is(err, bot.ErrorBadRequest) &&
		(strings.Contains(lower, "topic_deleted") ||
			strings.Contains(lower, "topic deleted") ||
			strings.Contains(lower, "message thread not found"))
}

func (a *app) forgetDeletedTelegramTopic(conv state.Conversation) error {
	return a.store.DeleteConversation(conv.ChatID, conv.MessageThreadID)
}
