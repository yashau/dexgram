package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dexgram/internal/config"
)

func TestTelegramChatIDCommandUpdatesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dexgram.toml")
	if err := os.WriteFile(path, []byte(`
[telegram]
bot_token = "token"
chat_ids = [111]
upload_final_answer_files = true

[codex]
cwd = "C:\\work"
approval_policy = "on-request"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runTelegramChatIDCommand([]string{"-config", path, "add", "-1001234567890"}, &out); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []int64{111, -1001234567890}
	if len(cfg.Telegram.ChatIDs) != len(wantIDs) {
		t.Fatalf("chat ids = %#v, want %#v", cfg.Telegram.ChatIDs, wantIDs)
	}
	for i := range wantIDs {
		if cfg.Telegram.ChatIDs[i] != wantIDs[i] {
			t.Fatalf("chat ids = %#v, want %#v", cfg.Telegram.ChatIDs, wantIDs)
		}
	}
	if cfg.Telegram.BotToken != "token" {
		t.Fatalf("bot token = %q, want token", cfg.Telegram.BotToken)
	}
	if !cfg.Telegram.UploadFinalAnswerFiles {
		t.Fatal("upload_final_answer_files was not preserved")
	}
	if cfg.Codex.CWD != `C:\work` || cfg.Codex.ApprovalPolicy != "on-request" {
		t.Fatalf("codex settings not preserved: %+v", cfg.Codex)
	}
	if !strings.Contains(out.String(), "Added Telegram chat id -1001234567890") {
		t.Fatalf("output did not confirm update: %q", out.String())
	}
}

func TestTelegramChatIDCommandRejectsInvalidID(t *testing.T) {
	var out bytes.Buffer
	if err := runTelegramChatIDCommand([]string{"add", "not-a-number"}, &out); err == nil {
		t.Fatal("expected invalid chat id error")
	}
}

func TestTelegramChatIDCommandRejectsWildcard(t *testing.T) {
	var out bytes.Buffer
	err := runTelegramChatIDCommand([]string{"add", "*"}, &out)
	if err == nil {
		t.Fatal("expected wildcard chat id error")
	}
	if !strings.Contains(err.Error(), "wildcard chat ids are not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTelegramChatIDCommandAddRequiresID(t *testing.T) {
	var out bytes.Buffer
	err := runTelegramChatIDCommand([]string{"add"}, &out)
	if err == nil {
		t.Fatal("expected missing chat id error")
	}
	if !strings.Contains(err.Error(), "retry as:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTelegramChatIDCommandRejectsZeroID(t *testing.T) {
	var out bytes.Buffer
	if err := runTelegramChatIDCommand([]string{"add", "0"}, &out); err == nil {
		t.Fatal("expected zero chat id error")
	}
}

func TestTelegramChatIDCommandDeletesOneID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dexgram.toml")
	if err := os.WriteFile(path, []byte(`
[telegram]
bot_token = "token"
chat_ids = [123, -100456]
`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runTelegramChatIDCommand([]string{"-config", path, "del", "123"}, &out); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Telegram.ChatIDs) != 1 || cfg.Telegram.ChatIDs[0] != -100456 {
		t.Fatalf("chat ids = %#v, want [-100456]", cfg.Telegram.ChatIDs)
	}
	if !strings.Contains(out.String(), "Deleted Telegram chat id 123") {
		t.Fatalf("expected delete confirmation, got %q", out.String())
	}
}

func TestTelegramChatIDCommandClearSetsDiscoveryMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dexgram.toml")
	if err := os.WriteFile(path, []byte(`
[telegram]
bot_token = "token"
chat_ids = [123, -100456]
`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runTelegramChatIDCommand([]string{"-config", path, "clear"}, &out); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Telegram.ChatIDs) != 0 {
		t.Fatalf("chat ids = %#v, want empty", cfg.Telegram.ChatIDs)
	}
	if !strings.Contains(out.String(), "Cleared all Telegram chat ids") {
		t.Fatalf("output did not confirm clear: %q", out.String())
	}
}

func TestParseTelegramChatIDActionDeleteAlias(t *testing.T) {
	action, chatID, err := parseTelegramChatIDAction([]string{"del", "-100456"})
	if err != nil {
		t.Fatal(err)
	}
	if action != telegramChatIDDelete || chatID != -100456 {
		t.Fatalf("parse del = (%q, %d), want (%q, -100456)", action, chatID, telegramChatIDDelete)
	}
}

func TestTelegramChatIDCommandRequiresExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")

	var out bytes.Buffer
	if err := runTelegramChatIDCommand([]string{"-config", path, "add", "123"}, &out); err == nil {
		t.Fatal("expected missing config error")
	}
}
