package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dexgram/internal/config"
	"dexgram/internal/state"
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

func TestRunTelegramCommandHelpAndUnknownSubcommand(t *testing.T) {
	if err := runTelegramCommand([]string{"--help"}); err != nil {
		t.Fatalf("telegram help command: %v", err)
	}
	if err := runTelegramCommand([]string{"unknown"}); err == nil {
		t.Fatal("expected unknown telegram subcommand to fail")
	}
}

func TestTelegramChatIDCommandRejectsInvalidID(t *testing.T) {
	var out bytes.Buffer
	if err := runTelegramChatIDCommand([]string{"add", "not-a-number"}, &out); err == nil {
		t.Fatal("expected invalid chat id or pairing code error")
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
	action, value, err := parseTelegramChatIDAction([]string{"del", "-100456"})
	if err != nil {
		t.Fatal(err)
	}
	if action != telegramChatIDDelete || value != "-100456" {
		t.Fatalf("parse del = (%q, %q), want (%q, -100456)", action, value, telegramChatIDDelete)
	}
}

func TestTelegramChatIDCommandRequiresExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")

	var out bytes.Buffer
	if err := runTelegramChatIDCommand([]string{"-config", path, "add", "123"}, &out); err == nil {
		t.Fatal("expected missing config error")
	}
}

func TestTelegramChatIDCommandAddsCaseInsensitivePairingCode(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "dexgram.toml")
	if err := os.WriteFile(configPath, []byte(`
[telegram]
bot_token = "token"
chat_ids = []
`), 0o600); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "dexgram.db")
	store, err := state.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTelegramPairingCode("ABC234", -100123, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	oldOpen := openTelegramPairingStore
	openTelegramPairingStore = func(string) (*state.Store, error) {
		return state.Open(dbPath)
	}
	t.Cleanup(func() {
		openTelegramPairingStore = oldOpen
	})

	var out bytes.Buffer
	if err := runTelegramChatIDCommand([]string{"-config", configPath, "add", "abc-234"}, &out); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Telegram.ChatIDs) != 1 || cfg.Telegram.ChatIDs[0] != -100123 {
		t.Fatalf("chat ids = %#v, want [-100123]", cfg.Telegram.ChatIDs)
	}

	store, err = state.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestStateStore(t, store)
	if _, ok, err := store.ConsumeTelegramPairingCode("ABC234"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("expected pairing code to be consumed")
	}
}

func TestNormalizeTelegramPairingCodeAcceptsDashedAndPlainLowercase(t *testing.T) {
	for _, value := range []string{"abc-234", "ABC234"} {
		got, err := normalizeTelegramPairingCode(value)
		if err != nil {
			t.Fatal(err)
		}
		if got != "ABC234" {
			t.Fatalf("normalizeTelegramPairingCode(%q) = %q, want ABC234", value, got)
		}
	}
}

func TestCreateTelegramPairingCodeSavesConsumableCode(t *testing.T) {
	store, err := state.Open(filepath.Join(t.TempDir(), "dexgram.db"))
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer closeTestStateStore(t, store)

	code, err := createTelegramPairingCode(store, -100123)
	if err != nil {
		t.Fatalf("create pairing code: %v", err)
	}
	formatted := formatTelegramPairingCode(code)
	if len(formatted) != 7 || formatted[3] != '-' {
		t.Fatalf("formatted pairing code = %q", formatted)
	}
	normalized, err := normalizeTelegramPairingCode(formatted)
	if err != nil {
		t.Fatalf("normalize pairing code: %v", err)
	}
	chatID, ok, err := store.ConsumeTelegramPairingCode(normalized)
	if err != nil {
		t.Fatalf("consume pairing code: %v", err)
	}
	if !ok || chatID != -100123 {
		t.Fatalf("consumed pairing code chat_id=%d ok=%v", chatID, ok)
	}
	if _, ok, err := store.ConsumeTelegramPairingCode(normalized); err != nil || ok {
		t.Fatalf("pairing code should be single use: ok=%v err=%v", ok, err)
	}
}

func TestTelegramTokenUpdatePromptsAndPreservesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dexgram.toml")
	if err := os.WriteFile(path, []byte(`
[telegram]
bot_token = "old-token"
chat_ids = [123, -100456]
upload_final_answer_files = true

[codex]
cwd = "C:\\work"
approval_policy = "on-request"
sandbox = "workspace-write"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runTelegramTokenCommand([]string{"-config", path, "update"}, strings.NewReader("new-token\n"), &out); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telegram.BotToken != "new-token" {
		t.Fatalf("bot token = %q, want new-token", cfg.Telegram.BotToken)
	}
	if len(cfg.Telegram.ChatIDs) != 2 || cfg.Telegram.ChatIDs[0] != 123 || cfg.Telegram.ChatIDs[1] != -100456 {
		t.Fatalf("chat ids not preserved: %#v", cfg.Telegram.ChatIDs)
	}
	if !cfg.Telegram.UploadFinalAnswerFiles || cfg.Codex.CWD != `C:\work` || cfg.Codex.ApprovalPolicy != "on-request" || cfg.Codex.Sandbox != "workspace-write" {
		t.Fatalf("config not preserved: %+v", cfg)
	}
	if !strings.Contains(out.String(), "Updated Telegram bot token") {
		t.Fatalf("output did not confirm token update: %q", out.String())
	}
}

func closeTestStateStore(t *testing.T, store *state.Store) {
	t.Helper()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}
