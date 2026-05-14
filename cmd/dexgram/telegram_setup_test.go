package main

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramCommandsIncludeUpdate(t *testing.T) {
	for _, command := range telegramCommands() {
		if command.Command == "update" {
			return
		}
	}
	t.Fatal("expected /update command to be registered")
}

func TestTelegramCommandClearScopesIncludeChatOnlyWhenRegistered(t *testing.T) {
	if got := telegramCommandClearScopes(nil); len(got) != 4 {
		t.Fatalf("empty chat_ids clear scopes count = %d, want 4", len(got))
	}

	got := telegramCommandClearScopes([]int64{123, -100456})
	if len(got) != 6 {
		t.Fatalf("registered clear scopes count = %d, want 6", len(got))
	}
	scope, ok := got[4].(*models.BotCommandScopeChat)
	if !ok {
		t.Fatalf("fifth scope = %T, want BotCommandScopeChat", got[4])
	}
	if scope.ChatID != int64(123) {
		t.Fatalf("chat command scope id = %#v, want 123", scope.ChatID)
	}
	scope, ok = got[5].(*models.BotCommandScopeChat)
	if !ok {
		t.Fatalf("sixth scope = %T, want BotCommandScopeChat", got[5])
	}
	if scope.ChatID != int64(-100456) {
		t.Fatalf("chat command scope id = %#v, want -100456", scope.ChatID)
	}
}
