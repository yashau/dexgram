package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dexgram/internal/config"
)

func TestRunOnboardWritesConfigFromPrompts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Dexgram", "dexgram.toml")
	in := strings.NewReader("123456:telegram-token\n2\nread-only\n")
	var out bytes.Buffer

	if err := runOnboard(in, &out, path); err != nil {
		t.Fatalf("run onboard: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load written config: %v", err)
	}
	if cfg.Telegram.BotToken != "123456:telegram-token" {
		t.Fatalf("bot token = %q", cfg.Telegram.BotToken)
	}
	if cfg.Codex.ApprovalPolicy != "on-request" {
		t.Fatalf("approval policy = %q", cfg.Codex.ApprovalPolicy)
	}
	if cfg.Codex.Sandbox != "read-only" {
		t.Fatalf("sandbox = %q", cfg.Codex.Sandbox)
	}
	if !strings.Contains(out.String(), "No Telegram chats are authorized yet.") {
		t.Fatalf("onboard output missing chat guidance: %q", out.String())
	}
}

func TestRunOnboardPreservesExistingConfigDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dexgram.toml")
	existing := strings.Join([]string{
		"[telegram]",
		`bot_token = "old-token"`,
		"chat_ids = [222, 111, 222]",
		"upload_final_answer_files = true",
		"",
		"[codex]",
		`cwd = "C:\\work"`,
		`cli_path = "C:\\Codex\\codex.exe"`,
		`approval_policy = "on-request"`,
		`sandbox = "workspace-write"`,
	}, "\n")
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	if err := runOnboard(strings.NewReader("\n\n\n"), io.Discard, path); err != nil {
		t.Fatalf("run onboard: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load written config: %v", err)
	}
	if cfg.Telegram.BotToken != "old-token" {
		t.Fatalf("bot token = %q", cfg.Telegram.BotToken)
	}
	if got := cfg.Telegram.ChatIDs; len(got) != 2 || got[0] != 222 || got[1] != 111 {
		t.Fatalf("chat ids = %#v", got)
	}
	if !cfg.Telegram.UploadFinalAnswerFiles {
		t.Fatal("upload final answer files was not preserved")
	}
	if cfg.Codex.CWD != `C:\work` || cfg.Codex.CLIPath != `C:\Codex\codex.exe` {
		t.Fatalf("codex paths = cwd %q cli %q", cfg.Codex.CWD, cfg.Codex.CLIPath)
	}
}

func TestPromptChoiceHandlesDefaultInvalidAndNamedChoices(t *testing.T) {
	choices := []onboardChoice{
		{Value: "first", Label: "First choice"},
		{Value: "second", Label: "Second choice"},
	}
	var out bytes.Buffer

	got, err := promptChoice(bufio.NewReader(strings.NewReader("bad\nSECOND\n")), &out, "Pick", choices, "first")
	if err != nil {
		t.Fatalf("prompt choice: %v", err)
	}
	if got != "second" {
		t.Fatalf("named choice = %q", got)
	}
	if !strings.Contains(out.String(), "Please choose a number") {
		t.Fatalf("expected invalid-choice guidance in %q", out.String())
	}

	got, err = promptChoice(bufio.NewReader(strings.NewReader("\n")), io.Discard, "Pick", choices, "second")
	if err != nil {
		t.Fatalf("prompt choice default: %v", err)
	}
	if got != "second" {
		t.Fatalf("default choice = %q", got)
	}
}

func TestOnboardSmallHelpers(t *testing.T) {
	if got, err := readPromptLine(bufio.NewReader(strings.NewReader("partial"))); err != nil || got != "partial" {
		t.Fatalf("partial EOF line = %q err=%v", got, err)
	}
	if got, err := readPromptLine(bufio.NewReader(strings.NewReader(""))); err != io.ErrUnexpectedEOF || got != "" {
		t.Fatalf("empty EOF line = %q err=%v", got, err)
	}
	if got := choiceIndex(approvalChoices, " ON-REQUEST "); got != 1 {
		t.Fatalf("choice index = %d", got)
	}
	if got := maskToken("short"); got != "********" {
		t.Fatalf("short masked token = %q", got)
	}
	if got := maskToken("1234567890"); got != "1234...7890" {
		t.Fatalf("long masked token = %q", got)
	}
}

func TestPrintOnboardHelpIncludesConfigPath(t *testing.T) {
	var out bytes.Buffer
	printOnboardHelp(&out, "dexgram.exe")
	text := out.String()
	if !strings.Contains(text, "dexgram.exe onboard") {
		t.Fatalf("onboard help missing usage: %q", text)
	}
	if !strings.Contains(text, "Dexgram") || !strings.Contains(text, "dexgram.toml") {
		t.Fatalf("onboard help missing config detail: %q", text)
	}
}

func TestRunOnboardCommandHelpAndUnknownArgument(t *testing.T) {
	if err := runOnboardCommand([]string{"--help"}); err != nil {
		t.Fatalf("onboard help command: %v", err)
	}
	if err := runOnboardCommand([]string{"unexpected"}); err == nil {
		t.Fatal("expected unknown onboard argument to fail")
	}
}
