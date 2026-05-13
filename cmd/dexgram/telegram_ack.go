package main

import (
	"context"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const telegramAckReactionTimeout = 5 * time.Second

var (
	telegramAckReactions = [...]string{"🗿", "🌚", "👾"}
	telegramAckSeq       atomic.Uint64
)

func ackTelegramMessage(ctx context.Context, b *bot.Bot, msg *models.Message) {
	if b == nil || !shouldAckTelegramMessage(msg) {
		return
	}

	chatID := msg.Chat.ID
	messageID := msg.ID
	reaction := nextTelegramAckReaction()

	go func() {
		actionCtx, cancel := context.WithTimeout(ctx, telegramAckReactionTimeout)
		defer cancel()

		_, err := b.SetMessageReaction(actionCtx, &bot.SetMessageReactionParams{
			ChatID:    chatID,
			MessageID: messageID,
			Reaction: []models.ReactionType{
				telegramEmojiReaction(reaction),
			},
		})
		if err != nil && actionCtx.Err() == nil {
			log.Printf("telegram ack reaction failed chat_id=%d message_id=%d reaction=%q: %v", chatID, messageID, reaction, err)
		}
	}()
}

func shouldAckTelegramMessage(msg *models.Message) bool {
	if msg == nil || msg.ID == 0 {
		return false
	}
	if strings.TrimSpace(msg.Text) != "" || strings.TrimSpace(msg.Caption) != "" {
		return true
	}
	return messageHasAttachment(msg)
}

func nextTelegramAckReaction() string {
	return pickTelegramAckReaction(telegramAckSeq.Add(1) - 1)
}

func pickTelegramAckReaction(index uint64) string {
	return telegramAckReactions[int(index%uint64(len(telegramAckReactions)))]
}

func telegramEmojiReaction(emoji string) models.ReactionType {
	return models.ReactionType{
		Type: models.ReactionTypeTypeEmoji,
		ReactionTypeEmoji: &models.ReactionTypeEmoji{
			Emoji: emoji,
		},
	}
}
