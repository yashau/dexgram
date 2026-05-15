package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"dexgram/internal/codex"
	"dexgram/internal/state"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	defaultSyncTurnLimit = 1
	maxSyncTurnLimit     = 5
	attachSyncMessages   = 25
	maxSyncRunLogLines   = 50
	maxSyncTextRunes     = 6000
)

func (a *app) handleSyncCommand(ctx context.Context, b *bot.Bot, msg *models.Message, arg string) {
	limit, err := parseSyncLimit(arg)
	if err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            err.Error(),
		})
		return
	}
	conv, ok, err := a.store.Get(msg.Chat.ID, msg.MessageThreadID)
	if err != nil {
		log.Printf("read conversation for sync: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Could not read Dexgram state: " + err.Error(),
		})
		return
	}
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
	resume, err := a.resumeCodexThreadResult(ctx, c, conv.CodexThreadID)
	if err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Could not resume Codex thread: " + err.Error()})
		return
	}
	thread := resume.Thread
	if len(thread.Turns) == 0 {
		var read codex.ThreadReadResponse
		if err := c.Call(ctx, "thread/read", map[string]any{"threadId": conv.CodexThreadID}, &read); err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Could not read Codex thread: " + err.Error()})
			return
		}
		thread = read.Thread
	}
	if len(thread.Turns) == 0 {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "No completed Codex turns to sync."})
		return
	}

	turns := unsyncedTurns(thread.Turns, conv.LastSyncedTurnID)
	if len(turns) == 0 {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Already synced."})
		return
	}
	if len(turns) > limit {
		turns = turns[len(turns)-limit:]
	}
	for _, turn := range turns {
		renderHistoricalTurn(ctx, b, msg.Chat.ID, msg.MessageThreadID, turn)
		conv.LastSyncedTurnID = turn.ID
	}
	if err := a.store.Upsert(conv); err != nil {
		log.Printf("store sync marker: %v", err)
	}
}

func parseSyncLimit(arg string) (int, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return defaultSyncTurnLimit, nil
	}
	n, err := strconv.Atoi(arg)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("Usage: /sync [1-%d]", maxSyncTurnLimit)
	}
	if n > maxSyncTurnLimit {
		return maxSyncTurnLimit, nil
	}
	return n, nil
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
	initial = truncateSyncText(initial)
	final = truncateSyncText(final)
	runLines = truncateRunLines(runLines)
	if strings.TrimSpace(initial) != "" {
		_ = sendSilentRichMessage(ctx, b, chatID, messageThreadID, initial)
	}
	if len(runLines) > 0 {
		text := "Synced run log\n\n" + strings.Join(runLines, "\n")
		rendered := firstRenderedTelegramMessage("```text\n"+text+"\n```", 3900)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:              chatID,
			MessageThreadID:     messageThreadID,
			Text:                rendered.Text,
			Entities:            rendered.Entities,
			DisableNotification: true,
		})
	}
	if strings.TrimSpace(final) != "" {
		_ = sendRichMessage(ctx, b, chatID, messageThreadID, final)
	} else {
		_ = sendRichMessage(ctx, b, chatID, messageThreadID, fmt.Sprintf("Synced Codex turn `%s`.", turn.ID))
	}
}

func (a *app) syncRecentAttachedHistory(ctx context.Context, b *bot.Bot, conv *state.Conversation) error {
	c, err := codex.StartStdioWithOptions(ctx, codex.StartOptions{
		CLIPath:    a.cfg.Codex.CLIPath,
		WorkingDir: appServerWorkingDir(*conv),
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = c.Close()
	}()
	go func() {
		for err := range c.Errors() {
			log.Printf("codex app-server: %v", err)
		}
	}()
	resume, err := a.resumeCodexThreadResult(ctx, c, conv.CodexThreadID)
	if err != nil {
		return err
	}
	thread := resume.Thread
	if len(thread.Turns) == 0 {
		var read codex.ThreadReadResponse
		if err := c.Call(ctx, "thread/read", map[string]any{"threadId": conv.CodexThreadID}, &read); err != nil {
			return err
		}
		thread = read.Thread
	}
	turns := recentCompletedTurnsByMessageBudget(thread.Turns, attachSyncMessages)
	log.Printf("attach sync thread_id=%s turns=%d selected=%d", conv.CodexThreadID, len(thread.Turns), len(turns))
	for _, turn := range turns {
		renderHistoricalTurn(ctx, b, conv.ChatID, conv.MessageThreadID, turn)
		conv.LastSyncedTurnID = turn.ID
	}
	if conv.LastSyncedTurnID != "" {
		if err := a.store.Upsert(*conv); err != nil {
			log.Printf("store attach sync marker: %v", err)
		}
	}
	return nil
}

func recentCompletedTurnsByMessageBudget(turns []codex.Turn, maxMessages int) []codex.Turn {
	if maxMessages <= 0 {
		return nil
	}
	var selected []codex.Turn
	count := 0
	for i := len(turns) - 1; i >= 0; i-- {
		turn := turns[i]
		if turn.Status != "completed" {
			continue
		}
		turnMessages := historicalTurnMessageCount(turn)
		if turnMessages == 0 {
			turnMessages = 1
		}
		if len(selected) > 0 && count+turnMessages > maxMessages {
			break
		}
		selected = append(selected, turn)
		count += turnMessages
		if count >= maxMessages {
			break
		}
	}
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}
	return selected
}

func historicalTurnMessageCount(turn codex.Turn) int {
	initial, runLines, final := summarizeTurn(turn)
	count := 0
	if strings.TrimSpace(initial) != "" {
		count++
	}
	if len(runLines) > 0 {
		count++
	}
	if strings.TrimSpace(final) != "" {
		count++
	} else {
		count++
	}
	return count
}

func truncateRunLines(lines []string) []string {
	if len(lines) <= maxSyncRunLogLines {
		return lines
	}
	out := append([]string(nil), lines[:maxSyncRunLogLines]...)
	out = append(out, fmt.Sprintf("... truncated %d more run log lines", len(lines)-maxSyncRunLogLines))
	return out
}

func truncateSyncText(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxSyncTextRunes {
		return string(runes)
	}
	return string(runes[:maxSyncTextRunes]) + "\n\n... truncated by /sync limit"
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
