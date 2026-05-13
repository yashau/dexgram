package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"dexgram/internal/codex"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (a *app) handleSyncCommand(ctx context.Context, b *bot.Bot, msg *models.Message) {
	conv, ok := a.store.Get(msg.Chat.ID, msg.MessageThreadID)
	if !ok || conv.CodexThreadID == "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "This topic is not mapped to a Codex thread yet.",
		})
		return
	}
	conv = a.topicConversation(msg.Chat.ID, msg.MessageThreadID)
	c, err := codex.StartStdioWithOptions(ctx, codex.StartOptions{
		CLIPath:    a.cfg.Codex.CLIPath,
		WorkingDir: appServerWorkingDir(conv),
	})
	if err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Could not start Codex app-server: " + err.Error()})
		return
	}
	defer func() {
		_ = c.Close()
	}()
	go func() {
		for err := range c.Errors() {
			log.Printf("codex app-server: %v", err)
		}
	}()
	if err := a.resumeCodexThread(ctx, c, conv.CodexThreadID); err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Could not resume Codex thread: " + err.Error()})
		return
	}
	var read codex.ThreadReadResponse
	if err := c.Call(ctx, "thread/read", map[string]any{"threadId": conv.CodexThreadID}, &read); err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Could not read Codex thread: " + err.Error()})
		return
	}

	turns := unsyncedTurns(read.Thread.Turns, conv.LastSyncedTurnID)
	if len(turns) == 0 {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Already synced."})
		return
	}
	if len(turns) > 5 {
		turns = turns[len(turns)-5:]
	}
	for _, turn := range turns {
		renderHistoricalTurn(ctx, b, msg.Chat.ID, msg.MessageThreadID, turn)
		conv.LastSyncedTurnID = turn.ID
	}
	if err := a.store.Upsert(conv); err != nil {
		log.Printf("store sync marker: %v", err)
	}
}

func unsyncedTurns(turns []codex.Turn, lastID string) []codex.Turn {
	if lastID == "" {
		for i := len(turns) - 1; i >= 0; i-- {
			if turns[i].Status == "completed" {
				return []codex.Turn{turns[i]}
			}
		}
		return nil
	}
	var out []codex.Turn
	seen := false
	for _, turn := range turns {
		if seen && turn.Status == "completed" {
			out = append(out, turn)
		}
		if turn.ID == lastID {
			seen = true
		}
	}
	return out
}

func renderHistoricalTurn(ctx context.Context, b *bot.Bot, chatID int64, messageThreadID int, turn codex.Turn) {
	initial, runLines, final := summarizeTurn(turn)
	if strings.TrimSpace(initial) != "" {
		_ = sendRichMessage(ctx, b, chatID, messageThreadID, initial)
	}
	if len(runLines) > 0 {
		text := "Synced run log\n\n" + strings.Join(runLines, "\n")
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: messageThreadID,
			Text:            "```text\n" + escapeCode(text) + "\n```",
			ParseMode:       models.ParseModeMarkdown,
		})
	}
	if strings.TrimSpace(final) != "" {
		_ = sendRichMessage(ctx, b, chatID, messageThreadID, final)
	} else {
		_ = sendRichMessage(ctx, b, chatID, messageThreadID, fmt.Sprintf("Synced Codex turn `%s`.", turn.ID))
	}
}

func summarizeTurn(turn codex.Turn) (string, []string, string) {
	var initial []string
	var runLines []string
	var lastAgent string
	var final string
	for _, item := range turn.Items {
		switch item.Type {
		case "agentMessage":
			lastAgent = item.Text
			if item.Phase != nil && *item.Phase == "final_answer" {
				final = item.Text
			} else if strings.TrimSpace(item.Text) != "" {
				initial = append(initial, item.Text)
			}
		case "plan":
			if strings.TrimSpace(item.Text) != "" {
				initial = append(initial, item.Text)
			}
		default:
			if line := runLogLine(item); line != "" {
				runLines = append(runLines, line)
			}
		}
	}
	if final == "" {
		final = lastAgent
	}
	return strings.Join(initial, "\n\n"), runLines, final
}
