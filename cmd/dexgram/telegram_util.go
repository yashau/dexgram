package main

import (
	"strings"

	"github.com/go-telegram/bot/models"
)

func turnControlMarkup(token string, queued bool) *models.InlineKeyboardMarkup {
	if token == "" {
		return nil
	}
	if queued {
		return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: "Steer", CallbackData: "st:" + token},
			{Text: "Delete queued", CallbackData: "dq:" + token},
		}}}
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "Stop", CallbackData: "sp:" + token},
	}}}
}

func messageHasAttachment(msg *models.Message) bool {
	return len(msg.Photo) > 0 || msg.Document != nil || msg.Animation != nil || msg.Audio != nil || msg.Video != nil || msg.Voice != nil
}

func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func splitNonEmptyLines(text string) []string {
	raw := strings.Split(text, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
