package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"dexgram/internal/codex"
	"dexgram/internal/config"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func reconcileCommands(ctx context.Context, b *bot.Bot, chatIDs []int64) error {
	chatIDs = config.NormalizeChatIDs(chatIDs)
	scopes := telegramCommandClearScopes(chatIDs)

	var errs []error
	for _, scope := range scopes {
		if _, err := b.DeleteMyCommands(ctx, &bot.DeleteMyCommandsParams{Scope: scope}); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if len(chatIDs) == 0 {
		log.Printf("telegram slash commands cleared; no registered chats yet")
		return nil
	}

	for _, chatID := range chatIDs {
		if _, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
			Commands: telegramCommands(),
			Scope:    &models.BotCommandScopeChat{ChatID: chatID},
		}); err != nil {
			return fmt.Errorf("register chat commands for %d: %w", chatID, err)
		}
	}
	log.Printf("telegram slash commands registered for chat_ids=%v", chatIDs)
	return nil
}

func telegramCommandClearScopes(chatIDs []int64) []models.BotCommandScope {
	scopes := []models.BotCommandScope{
		&models.BotCommandScopeDefault{},
		&models.BotCommandScopeAllPrivateChats{},
		&models.BotCommandScopeAllGroupChats{},
		&models.BotCommandScopeAllChatAdministrators{},
	}
	for _, chatID := range config.NormalizeChatIDs(chatIDs) {
		scopes = append(scopes, &models.BotCommandScopeChat{ChatID: chatID})
	}
	return scopes
}

func telegramCommands() []models.BotCommand {
	return []models.BotCommand{
		{Command: "project", Description: "Set the Codex project before this chat starts"},
		{Command: "new", Description: "Create a new Telegram topic for a Codex chat"},
		{Command: "side", Description: "Fork this Codex chat into a side topic"},
		{Command: "status", Description: "Show this topic's Dexgram mapping and turn state"},
		{Command: "sync", Description: "Mirror completed Codex turns into this topic"},
		{Command: "update", Description: "Update Dexgram and restart the bridge"},
		{Command: "steer", Description: "Steer the active Codex turn"},
		{Command: "goal", Description: "Set the Codex goal for this topic"},
		{Command: "plan", Description: "Start a Codex Plan Mode turn"},
		{Command: "settings", Description: "Show Telegram-started Plan Mode settings"},
		{Command: "model", Description: "Choose the model for Plan Mode"},
		{Command: "effort", Description: "Choose the reasoning effort for Plan Mode"},
		{Command: "stop", Description: "Stop the active Codex turn"},
		{Command: "cancel", Description: "Alias for /stop"},
	}
}

func ensureThreadedMode(ctx context.Context, b *bot.Bot, me *models.User, chatIDs []int64) error {
	var missing []string
	if !me.HasTopicsEnabled {
		missing = append(missing, "Threaded Mode")
	}
	if !me.AllowsUsersToCreateTopics {
		missing = append(missing, "users creating topics")
	}
	if len(missing) == 0 {
		log.Printf("telegram threaded mode is enabled")
		return nil
	}

	message := threadedModeSetupMessage(me.Username, missing)
	for _, chatID := range config.NormalizeChatIDs(chatIDs) {
		if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   message,
		}); err != nil {
			log.Printf("could not send threaded-mode setup guidance to chat_id=%d: %v", chatID, err)
		}
	}
	return fmt.Errorf("telegram bot is not ready for private chat topics: %s", message)
}

func threadedModeSetupMessage(username string, missing []string) string {
	name := "@BotFather"
	if username != "" {
		name = fmt.Sprintf("@%s via @BotFather", username)
	}
	return fmt.Sprintf(
		"Dexgram needs Telegram threaded mode before it can create per-project topics.\n\n"+
			"Missing: %s.\n\n"+
			"Open @BotFather, choose %s, then go to Bot Settings -> Threads Settings and enable Threaded Mode. "+
			"Also leave user-created topics enabled. Restart Dexgram after saving the setting.",
		strings.Join(missing, ", "),
		name,
	)
}

func threadTitle(t codex.Thread) string {
	if strings.TrimSpace(t.Name) != "" {
		return strings.TrimSpace(t.Name)
	}
	if strings.TrimSpace(t.Preview) != "" {
		return strings.TrimSpace(t.Preview)
	}
	return ""
}

func topicTitle(projectName, chatName string) string {
	projectName = strings.TrimSpace(projectName)
	chatName = strings.TrimSpace(chatName)
	if projectName == "" || strings.EqualFold(projectName, "One-off") {
		return truncateTopicPart(chatName, 32)
	}
	if chatName == "" {
		return truncateTopicPart(projectName, 32)
	}
	return balancedTopicTitle(projectName, chatName, 32)
}

func sideTopicTitle(base string, index int) string {
	base = compactSpaces(base)
	if base == "" {
		base = "Side chat"
	}
	var prefix string
	if index > 0 {
		prefix = fmt.Sprintf("↳%d ", index)
	} else {
		prefix = "↳ "
	}
	budget := 32 - runeLen(prefix)
	if budget <= 0 {
		return truncateTopicPart(prefix, 32)
	}
	return prefix + truncateTopicPart(base, budget)
}

func balancedTopicTitle(projectName, chatName string, max int) string {
	const sep = ": "
	projectName = compactSpaces(projectName)
	chatName = compactSpaces(chatName)
	if runeLen(projectName)+len(sep)+runeLen(chatName) <= max {
		return projectName + sep + chatName
	}
	budget := max - len(sep)
	if budget < 5 {
		return truncateTopicPart(projectName+sep+chatName, max)
	}
	projectBudget := budget / 2
	chatBudget := budget - projectBudget
	if runeLen(projectName) < projectBudget {
		chatBudget += projectBudget - runeLen(projectName)
		projectBudget = runeLen(projectName)
	}
	if runeLen(chatName) < chatBudget {
		projectBudget += chatBudget - runeLen(chatName)
		chatBudget = runeLen(chatName)
	}
	return truncateTopicPart(projectName, projectBudget) + sep + truncateTopicPart(chatName, chatBudget)
}

func truncateTopicPart(s string, max int) string {
	s = compactSpaces(s)
	if max <= 0 {
		return ""
	}
	if runeLen(s) <= max {
		return s
	}
	if max <= 3 {
		return string([]rune(s)[:max])
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}

func compactSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func runeLen(s string) int {
	return len([]rune(s))
}
