package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsFinalAnswerFileUploadsDisabled(t *testing.T) {
	path := writeTestConfig(t, `
[telegram]
bot_token = "token"
chat_id = 123
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Telegram.UploadFinalAnswerFiles {
		t.Fatal("expected final-answer file uploads to default disabled")
	}
}

func TestLoadFinalAnswerFileUploadsEnabled(t *testing.T) {
	path := writeTestConfig(t, `
[telegram]
bot_token = "token"
chat_id = 123
upload_final_answer_files = true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.Telegram.UploadFinalAnswerFiles {
		t.Fatal("expected final-answer file uploads to be enabled")
	}
}

func TestLoadTrimsValuesAndAppliesCodexDefaults(t *testing.T) {
	path := writeTestConfig(t, `
[telegram]
bot_token = "  token  "
chat_id = 123

[codex]
cwd = "  C:\\work\\dexgram  "
cli_path = "  C:\\codex\\codex.exe  "
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Telegram.BotToken != "token" {
		t.Fatalf("bot token was not trimmed: %q", cfg.Telegram.BotToken)
	}
	if cfg.Codex.CWD != `C:\work\dexgram` {
		t.Fatalf("cwd was not trimmed: %q", cfg.Codex.CWD)
	}
	if cfg.Codex.CLIPath != `C:\codex\codex.exe` {
		t.Fatalf("cli path was not trimmed: %q", cfg.Codex.CLIPath)
	}
	if cfg.Codex.ApprovalPolicy != "never" {
		t.Fatalf("approval policy default = %q", cfg.Codex.ApprovalPolicy)
	}
	if cfg.Codex.Sandbox != "danger-full-access" {
		t.Fatalf("sandbox default = %q", cfg.Codex.Sandbox)
	}
}

func TestLoadPreservesExplicitCodexSettings(t *testing.T) {
	path := writeTestConfig(t, `
[telegram]
bot_token = "token"

[codex]
approval_policy = " on-request "
sandbox = " workspace-write "
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Codex.ApprovalPolicy != "on-request" {
		t.Fatalf("approval policy = %q", cfg.Codex.ApprovalPolicy)
	}
	if cfg.Codex.Sandbox != "workspace-write" {
		t.Fatalf("sandbox = %q", cfg.Codex.Sandbox)
	}
}

func TestLoadRequiresTelegramBotToken(t *testing.T) {
	path := writeTestConfig(t, `
[telegram]
chat_id = 123
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected missing bot token error")
	}
}

func TestLoadReportsMissingConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")

	if _, err := Load(path); err == nil {
		t.Fatal("expected missing file error")
	}
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dexgram.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
