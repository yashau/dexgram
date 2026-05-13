package main

import (
	"strings"
	"testing"

	"dexgram/internal/config"
)

func TestParseTelegramCommand(t *testing.T) {
	tests := []struct {
		text   string
		name   string
		arg    string
		wantOK bool
	}{
		{text: "/project dexgram", name: "project", arg: "dexgram", wantOK: true},
		{text: " /NEW@DexgramBot title ", name: "new", arg: "title", wantOK: true},
		{text: "hello", wantOK: false},
		{text: "/", wantOK: false},
	}

	for _, test := range tests {
		name, arg, ok := parseTelegramCommand(test.text)
		if name != test.name || arg != test.arg || ok != test.wantOK {
			t.Fatalf("parseTelegramCommand(%q) = %q, %q, %v", test.text, name, arg, ok)
		}
	}
}

func TestAllowedChat(t *testing.T) {
	app := &app{cfg: &config.Config{}}
	app.cfg.Telegram.ChatID = 0
	if !app.allowedChat(123) {
		t.Fatal("chat_id 0 should allow all chats")
	}
	app.cfg.Telegram.ChatID = 123
	if !app.allowedChat(123) || app.allowedChat(456) {
		t.Fatal("allowedChat did not enforce configured chat id")
	}
}

func TestGoalDisabledMessageDetection(t *testing.T) {
	if !isCodexGoalsDisabledError(assertErr("Goals feature is disabled by config")) {
		t.Fatal("expected disabled goals error to be recognized")
	}
	if isCodexGoalsDisabledError(nil) || isCodexGoalsDisabledError(assertErr("other error")) {
		t.Fatal("unexpected goals disabled recognition")
	}
	if msg := codexGoalsDisabledMessage(); !strings.Contains(msg, "[features]") || !strings.Contains(msg, "goals = true") {
		t.Fatalf("unexpected goals message: %q", msg)
	}
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}
