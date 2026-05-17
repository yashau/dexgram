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
	if app.allowedChat(123) {
		t.Fatal("empty chat_ids should leave chats unregistered")
	}
	app.cfg.Telegram.ChatIDs = []int64{123, -100456}
	if !app.allowedChat(123) || !app.allowedChat(-100456) || app.allowedChat(456) {
		t.Fatal("allowedChat did not enforce configured chat ids")
	}
}

func TestUnregisteredChatMessageIncludesReadyCommand(t *testing.T) {
	got := unregisteredChatMessage("ABC234", "")
	if !strings.Contains(got, "Pairing code:\nABC-234") {
		t.Fatalf("message did not include pairing code: %q", got)
	}
	if !strings.Contains(got, "dexgram telegram chatid add ABC-234") {
		t.Fatalf("message did not include ready command: %q", got)
	}
	if strings.Contains(got, "chat_id") {
		t.Fatalf("unregistered chat message should not expose chat id: %q", got)
	}
	if strings.Contains(got, "Codex") {
		t.Fatalf("unregistered chat message should not route user toward Codex: %q", got)
	}
}

func TestTelegramChatIDCommandIncludesConfigWhenCustom(t *testing.T) {
	got := telegramChatIDCommand("ABC-234", `C:\Dexgram\dexgram.toml`)
	want := `dexgram telegram chatid -config 'C:\Dexgram\dexgram.toml' add ABC-234`
	if got != want {
		t.Fatalf("telegramChatIDCommand() = %q, want %q", got, want)
	}
}

func TestQuotePowerShellArgEscapesSingleQuotes(t *testing.T) {
	got := quotePowerShellArg(`C:\Users\O'Brien\dexgram.toml`)
	want := `'C:\Users\O''Brien\dexgram.toml'`
	if got != want {
		t.Fatalf("quotePowerShellArg() = %q, want %q", got, want)
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

func TestTelegramNoopTopicEditDetection(t *testing.T) {
	for _, err := range []error{
		assertErr("Bad Request: TOPIC_NOT_MODIFIED"),
		assertErr("Bad Request: forum topic not modified"),
	} {
		if !isTelegramNoopTopicEdit(err) {
			t.Fatalf("expected noop topic edit error: %v", err)
		}
	}
	if isTelegramNoopTopicEdit(nil) || isTelegramNoopTopicEdit(assertErr("Bad Request: topic deleted")) {
		t.Fatal("unexpected noop topic edit detection")
	}
}

func TestGoalClearCommandAliases(t *testing.T) {
	for _, text := range []string{"clear", "delete", "remove", "stop", "off", "none", " CLEAR "} {
		if !isGoalClearCommand(text) {
			t.Fatalf("expected %q to clear goal", text)
		}
	}
	for _, text := range []string{"", "pause", "resume", "clear database migration", "pause work until tomorrow"} {
		if isGoalClearCommand(text) {
			t.Fatalf("did not expect %q to clear goal", text)
		}
	}
}

func TestGoalPauseAndResumeCommands(t *testing.T) {
	if !isGoalPauseCommand("pause") || !isGoalPauseCommand(" PAUSE ") {
		t.Fatal("expected pause command to be recognized")
	}
	if isGoalPauseCommand("pause work until tomorrow") {
		t.Fatal("multi-word goal should not be treated as pause command")
	}
	if !isGoalResumeCommand("resume") || !isGoalResumeCommand(" RESUME ") {
		t.Fatal("expected resume command to be recognized")
	}
	if isGoalResumeCommand("resume migration goal") {
		t.Fatal("multi-word goal should not be treated as resume command")
	}
}

func TestGoalStatusAndHelpCommands(t *testing.T) {
	for _, text := range []string{"status", "show", "current", "get"} {
		if !isGoalStatusCommand(text) {
			t.Fatalf("expected %q to show goal status", text)
		}
	}
	if isGoalStatusCommand("status report") {
		t.Fatal("multi-word goal should not be treated as status command")
	}
	for _, text := range []string{"help", "?"} {
		if !isGoalHelpCommand(text) {
			t.Fatalf("expected %q to show goal help", text)
		}
	}
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}
