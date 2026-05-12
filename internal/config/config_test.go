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

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dexgram.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
