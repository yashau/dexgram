package main

import (
	"strings"
	"testing"

	"dexgram/internal/codex"

	"github.com/go-telegram/bot/models"
)

func TestTelegramCommandsIncludeUpdateSideBTWAndUsage(t *testing.T) {
	found := map[string]bool{}
	for _, command := range telegramCommands() {
		found[command.Command] = true
	}
	for _, command := range []string{"update", "side", "btw", "usage"} {
		if !found[command] {
			t.Fatalf("expected /%s command to be registered", command)
		}
	}
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

func TestThreadedModeSetupMessageNamesMissingSettings(t *testing.T) {
	got := threadedModeSetupMessage("dexgram_bot", []string{"Threaded Mode", "users creating topics"})
	if !strings.Contains(got, "@dexgram_bot via @BotFather") {
		t.Fatalf("message missing bot name: %q", got)
	}
	if !strings.Contains(got, "Missing: Threaded Mode, users creating topics.") {
		t.Fatalf("message missing settings: %q", got)
	}

	got = threadedModeSetupMessage("", []string{"Threaded Mode"})
	if !strings.Contains(got, "choose @BotFather") {
		t.Fatalf("message should fall back to BotFather: %q", got)
	}
}

func TestTopicTitleBalancesAndTruncatesParts(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		chatName    string
		want        string
	}{
		{
			name:        "one off uses chat name",
			projectName: " One-off ",
			chatName:    "  Quick   question  ",
			want:        "Quick question",
		},
		{
			name:        "missing chat uses project",
			projectName: "Dexgram",
			chatName:    "",
			want:        "Dexgram",
		},
		{
			name:        "balanced title",
			projectName: "Very Long Project Name For Dexgram",
			chatName:    "Very Long Chat Topic Name",
			want:        "Very Long Proj\u2026: Very Long Chat\u2026",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := topicTitle(test.projectName, test.chatName); got != test.want {
				t.Fatalf("topic title = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTelegramTopicTitleHelpers(t *testing.T) {
	if got := threadTitle(codex.Thread{Name: "  Named topic  ", Preview: "preview"}); got != "Named topic" {
		t.Fatalf("thread title name = %q", got)
	}
	if got := threadTitle(codex.Thread{Preview: "  Preview only  "}); got != "Preview only" {
		t.Fatalf("thread title preview = %q", got)
	}
	if got := threadTitle(codex.Thread{}); got != "" {
		t.Fatalf("empty thread title = %q", got)
	}
	if got := truncateTopicPart("abcdef", 3); got != "abc" {
		t.Fatalf("short truncation = %q", got)
	}
	if got := truncateTopicPart("abcdef", 5); got != "abcd\u2026" {
		t.Fatalf("ellipsis truncation = %q", got)
	}
	if got := truncateTopicPart("abcdef", 0); got != "" {
		t.Fatalf("zero truncation = %q", got)
	}
	if got := compactSpaces(" alpha\t beta \n gamma "); got != "alpha beta gamma" {
		t.Fatalf("compact spaces = %q", got)
	}
	if got := runeLen("a\u2026b"); got != 3 {
		t.Fatalf("rune len = %d", got)
	}
	if got := sideTopicTitle("Dexgram: auth flow", 2); got != "↳2 Dexgram: auth flow" {
		t.Fatalf("side topic title = %q", got)
	}
	if got := sideTopicTitle("A very long parent title that cannot fit", 12); got != "↳12 A very long parent title th…" {
		t.Fatalf("long side topic title = %q", got)
	}
}
