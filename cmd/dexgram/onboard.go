package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dexgram/internal/config"

	"github.com/BurntSushi/toml"
)

type onboardChoice struct {
	Value string
	Label string
}

var approvalChoices = []onboardChoice{
	{Value: "never", Label: "Full access: do not ask before commands/tools"},
	{Value: "on-request", Label: "Ask when Codex needs approval"},
}

var sandboxChoices = []onboardChoice{
	{Value: "danger-full-access", Label: "No filesystem sandbox"},
	{Value: "workspace-write", Label: "Write in the workspace, restrict outside"},
	{Value: "read-only", Label: "No filesystem writes without approval"},
}

func runOnboardCommand(args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		printOnboardHelp(os.Stdout, filepath.Base(os.Args[0]))
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("unknown onboard argument %q; run %s onboard --help", args[0], filepath.Base(os.Args[0]))
	}
	return runOnboard(os.Stdin, os.Stdout, mustServiceConfigPath())
}

func printOnboardHelp(w io.Writer, exe string) {
	_, _ = fmt.Fprintf(w, `Dexgram Onboarding

Usage

  %[1]s onboard

Creates or updates:

  %[2]s

Dexgram will ask for the Telegram bot token/id, Codex approval policy, and
Codex sandbox mode. Existing chat_ids, cwd, and cli_path values are preserved.

`, exe, mustServiceConfigPath())
}

func runOnboard(in io.Reader, out io.Writer, path string) error {
	reader := bufio.NewReader(in)
	cfg, err := loadOnboardConfig(path)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out, "Dexgram onboarding")
	_, _ = fmt.Fprintf(out, "Config: %s\n\n", path)

	botToken, err := promptRequired(reader, out, "Telegram bot token/id from @BotFather", cfg.Telegram.BotToken)
	if err != nil {
		return err
	}
	approvalPolicy, err := promptChoice(reader, out, "Codex approval mode", approvalChoices, cfg.Codex.ApprovalPolicy)
	if err != nil {
		return err
	}
	sandbox, err := promptChoice(reader, out, "Codex sandbox mode", sandboxChoices, cfg.Codex.Sandbox)
	if err != nil {
		return err
	}

	cfg.Telegram.BotToken = botToken
	cfg.Codex.ApprovalPolicy = approvalPolicy
	cfg.Codex.Sandbox = sandbox
	applyOnboardDefaults(&cfg)

	if err := writeOnboardConfig(path, cfg); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "Wrote %s\n", path)
	if len(cfg.Telegram.ChatIDs) == 0 {
		_, _ = fmt.Fprintln(out, "No Telegram chats are authorized yet.")
		_, _ = fmt.Fprintln(out, "Start Dexgram, then DM the bot or add it to a group and send a message.")
		_, _ = fmt.Fprintln(out, "The bot will reply in Telegram with a dexgram telegram chatid add command to copy into this terminal.")
	}
	_, _ = fmt.Fprintf(out, "Next: %s -check\n", filepath.Base(os.Args[0]))
	return nil
}

func loadOnboardConfig(path string) (config.Config, error) {
	var cfg config.Config
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			applyOnboardDefaults(&cfg)
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config metadata: %w", err)
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("decode existing config %s: %w", path, err)
	}
	applyOnboardDefaults(&cfg)
	return cfg, nil
}

func applyOnboardDefaults(cfg *config.Config) {
	cfg.Telegram.BotToken = strings.TrimSpace(cfg.Telegram.BotToken)
	if len(cfg.Telegram.ChatIDs) == 0 && cfg.Telegram.ChatID != 0 {
		cfg.Telegram.ChatIDs = []int64{cfg.Telegram.ChatID}
	}
	cfg.Telegram.ChatIDs = config.NormalizeChatIDs(cfg.Telegram.ChatIDs)
	cfg.Codex.CWD = strings.TrimSpace(cfg.Codex.CWD)
	cfg.Codex.CLIPath = strings.TrimSpace(cfg.Codex.CLIPath)
	cfg.Codex.ApprovalPolicy = strings.TrimSpace(cfg.Codex.ApprovalPolicy)
	cfg.Codex.Sandbox = strings.TrimSpace(cfg.Codex.Sandbox)
	if cfg.Codex.ApprovalPolicy == "" {
		cfg.Codex.ApprovalPolicy = approvalChoices[0].Value
	}
	if cfg.Codex.Sandbox == "" {
		cfg.Codex.Sandbox = sandboxChoices[0].Value
	}
}

func promptRequired(reader *bufio.Reader, out io.Writer, label, current string) (string, error) {
	for {
		if current == "" {
			_, _ = fmt.Fprintf(out, "%s: ", label)
		} else {
			_, _ = fmt.Fprintf(out, "%s [%s]: ", label, maskToken(current))
		}
		value, err := readPromptLine(reader)
		if err != nil {
			return "", err
		}
		if value == "" {
			value = current
		}
		value = strings.TrimSpace(value)
		if value != "" {
			return value, nil
		}
		_, _ = fmt.Fprintln(out, "Please enter a value.")
	}
}

func promptChoice(reader *bufio.Reader, out io.Writer, label string, choices []onboardChoice, current string) (string, error) {
	defaultIndex := choiceIndex(choices, current)
	if defaultIndex < 0 {
		defaultIndex = 0
	}
	for {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "%s:\n", label)
		for i, choice := range choices {
			suffix := ""
			if i == defaultIndex {
				suffix = " (default)"
			}
			_, _ = fmt.Fprintf(out, "  %d) %s%s - %s\n", i+1, choice.Value, suffix, choice.Label)
		}
		_, _ = fmt.Fprintf(out, "Choose 1-%d [%d]: ", len(choices), defaultIndex+1)
		value, err := readPromptLine(reader)
		if err != nil {
			return "", err
		}
		if value == "" {
			return choices[defaultIndex].Value, nil
		}
		if n, err := strconv.Atoi(value); err == nil && n >= 1 && n <= len(choices) {
			return choices[n-1].Value, nil
		}
		if idx := choiceIndex(choices, value); idx >= 0 {
			return choices[idx].Value, nil
		}
		_, _ = fmt.Fprintf(out, "Please choose a number from 1 to %d, or enter one of the shown values.\n", len(choices))
	}
}

func readPromptLine(reader *bufio.Reader) (string, error) {
	value, err := reader.ReadString('\n')
	if errors.Is(err, io.EOF) {
		value = strings.TrimSpace(value)
		if value == "" {
			return "", io.ErrUnexpectedEOF
		}
		return value, nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func choiceIndex(choices []onboardChoice, value string) int {
	value = strings.TrimSpace(value)
	for i, choice := range choices {
		if strings.EqualFold(choice.Value, value) {
			return i
		}
	}
	return -1
}

func maskToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return "********"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

func writeOnboardConfig(path string, cfg config.Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open config for writing: %w", err)
	}
	encErr := toml.NewEncoder(f).Encode(configForWrite(cfg))
	closeErr := f.Close()
	if encErr != nil {
		return fmt.Errorf("encode config: %w", encErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close config: %w", closeErr)
	}
	return nil
}

type writableConfig struct {
	Telegram writableTelegramConfig `toml:"telegram"`
	Codex    config.CodexConfig     `toml:"codex"`
}

type writableTelegramConfig struct {
	BotToken               string  `toml:"bot_token"`
	ChatIDs                []int64 `toml:"chat_ids"`
	UploadFinalAnswerFiles bool    `toml:"upload_final_answer_files"`
}

func configForWrite(cfg config.Config) writableConfig {
	return writableConfig{
		Telegram: writableTelegramConfig{
			BotToken:               cfg.Telegram.BotToken,
			ChatIDs:                config.NormalizeChatIDs(cfg.Telegram.ChatIDs),
			UploadFinalAnswerFiles: cfg.Telegram.UploadFinalAnswerFiles,
		},
		Codex: cfg.Codex,
	}
}
