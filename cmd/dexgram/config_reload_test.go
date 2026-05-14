package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dexgram/internal/config"
)

func TestReloadConfigIfChangedReloadsTokenAndCodexSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dexgram.toml")
	if err := os.WriteFile(path, []byte(`
[telegram]
bot_token = "token-one"
chat_ids = [123]

[codex]
approval_policy = "never"
sandbox = "danger-full-access"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := statConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	app := &app{cfg: cfg, configPath: path, configState: state}

	if err := os.WriteFile(path, []byte(`
[telegram]
bot_token = "token-two"
chat_ids = [123]
upload_final_answer_files = true

[codex]
approval_policy = "on-request"
sandbox = "workspace-write"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	nextModTime := time.Now().Add(time.Second)
	if err := os.Chtimes(path, nextModTime, nextModTime); err != nil {
		t.Fatal(err)
	}

	result := app.reloadConfigIfChanged(context.Background(), nil)
	if !result.reloaded || !result.botTokenChanged {
		t.Fatalf("reload result = %#v, want token reload", result)
	}
	if app.cfg.Telegram.BotToken != "token-two" {
		t.Fatalf("bot token = %q, want token-two", app.cfg.Telegram.BotToken)
	}
	if !app.cfg.Telegram.UploadFinalAnswerFiles {
		t.Fatal("upload_final_answer_files was not reloaded")
	}
	if app.cfg.Codex.ApprovalPolicy != "on-request" || app.cfg.Codex.Sandbox != "workspace-write" {
		t.Fatalf("codex config was not reloaded: %+v", app.cfg.Codex)
	}
}
