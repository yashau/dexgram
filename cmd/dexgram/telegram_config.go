package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dexgram/internal/config"
	"dexgram/internal/state"
)

var openTelegramPairingStore = state.Open

func runTelegramCommand(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printTelegramHelp(os.Stdout, filepath.Base(os.Args[0]))
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "chatid", "chat-id", "id":
		return runTelegramChatIDCommand(args[1:], os.Stdout)
	case "token", "bot-token":
		return runTelegramTokenCommand(args[1:], os.Stdin, os.Stdout)
	default:
		return fmt.Errorf("unknown telegram command %q; run %s telegram --help", args[0], filepath.Base(os.Args[0]))
	}
}

func printTelegramHelp(w io.Writer, exe string) {
	_, _ = fmt.Fprintf(w, `Dexgram Telegram Commands

Usage

  %[1]s telegram chatid add <chat_id_or_pairing_code>
  %[1]s telegram chatid del <chat_id>
  %[1]s telegram chatid clear
  %[1]s telegram token update
  %[1]s tg id add <chat_id_or_pairing_code>

Options

  -config path
      Path to Dexgram TOML config. Defaults to .\dexgram.toml if present,
      otherwise %%APPDATA%%\Dexgram\dexgram.toml.

Examples

  %[1]s telegram chatid add 123456789
  %[1]s telegram chatid add -1001234567890
  %[1]s telegram chatid add ABC-234
  %[1]s telegram chatid add abc234
  %[1]s telegram chatid del 123456789
  %[1]s telegram chatid clear
  %[1]s telegram token update

`, exe)
}

func runTelegramChatIDCommand(args []string, out io.Writer) error {
	var configPath string
	fs := flag.NewFlagSet("telegram chatid", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&configPath, "config", defaultConfigPath(), "path to Dexgram TOML config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	action, value, err := parseTelegramChatIDAction(fs.Args())
	if err != nil {
		return err
	}
	chatID, err := resolveTelegramChatIDAction(action, value)
	if err != nil {
		return err
	}

	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("config file not found: %s; run %s onboard first", configPath, filepath.Base(os.Args[0]))
		}
		return fmt.Errorf("read config metadata: %w", err)
	}
	cfg, err := loadOnboardConfig(configPath)
	if err != nil {
		return err
	}
	cfg.Telegram.ChatIDs = applyTelegramChatIDAction(cfg.Telegram.ChatIDs, action, chatID)
	cfg.Telegram.ChatID = 0
	if err := writeOnboardConfig(configPath, cfg); err != nil {
		return err
	}

	switch action {
	case telegramChatIDAdd:
		_, _ = fmt.Fprintf(out, "Added Telegram chat id %d in %s\n", chatID, configPath)
	case telegramChatIDDelete:
		_, _ = fmt.Fprintf(out, "Deleted Telegram chat id %d in %s\n", chatID, configPath)
	case telegramChatIDClear:
		_, _ = fmt.Fprintf(out, "Cleared all Telegram chat ids in %s\n", configPath)
	}
	if len(cfg.Telegram.ChatIDs) == 0 {
		_, _ = fmt.Fprintln(out, "Warning: chat_ids is empty; no Telegram chats are authorized. Unauthorized chats will only receive setup instructions.")
	}
	return nil
}

type telegramChatIDAction string

const (
	telegramChatIDAdd    telegramChatIDAction = "add"
	telegramChatIDDelete telegramChatIDAction = "delete"
	telegramChatIDClear  telegramChatIDAction = "clear"
)

func parseTelegramChatIDAction(args []string) (telegramChatIDAction, string, error) {
	if len(args) == 0 {
		return "", "", fmt.Errorf("usage: %s telegram chatid <add|del|clear> [chat_id]", filepath.Base(os.Args[0]))
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "add":
		if len(args) != 2 {
			return "", "", fmt.Errorf("missing chat id; retry as: %s telegram chatid add <chat_id_or_pairing_code>", filepath.Base(os.Args[0]))
		}
		return telegramChatIDAdd, strings.TrimSpace(args[1]), nil
	case "del", "delete", "rm", "remove":
		if len(args) != 2 {
			return "", "", fmt.Errorf("missing chat id; retry as: %s telegram chatid del <chat_id>", filepath.Base(os.Args[0]))
		}
		return telegramChatIDDelete, strings.TrimSpace(args[1]), nil
	case "clear":
		if len(args) != 1 {
			return "", "", fmt.Errorf("usage: %s telegram chatid clear", filepath.Base(os.Args[0]))
		}
		return telegramChatIDClear, "", nil
	default:
		return "", "", fmt.Errorf("unknown chatid action %q; use add, del, or clear", args[0])
	}
}

func resolveTelegramChatIDAction(action telegramChatIDAction, value string) (int64, error) {
	switch action {
	case telegramChatIDAdd:
		if chatID, err := parseTelegramChatID(value); err == nil {
			return chatID, nil
		} else if strings.TrimSpace(value) == "*" || isSignedInteger(value) {
			return 0, err
		}
		code, err := normalizeTelegramPairingCode(value)
		if err != nil {
			return 0, err
		}
		store, err := openTelegramPairingStore("")
		if err != nil {
			return 0, fmt.Errorf("open Dexgram state to resolve Telegram pairing code: %w", err)
		}
		defer func() {
			_ = store.Close()
		}()
		chatID, ok, err := store.ConsumeTelegramPairingCode(code)
		if err != nil {
			return 0, fmt.Errorf("resolve Telegram pairing code: %w", err)
		}
		if !ok {
			return 0, fmt.Errorf("Telegram pairing code %s was not found or has expired; send another message to the bot and retry", formatTelegramPairingCode(code))
		}
		return chatID, nil
	case telegramChatIDDelete:
		return parseTelegramChatID(value)
	case telegramChatIDClear:
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown chatid action %q", action)
	}
}

func isSignedInteger(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if value[0] == '-' || value[0] == '+' {
		value = value[1:]
	}
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseTelegramChatID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "*" {
		return 0, fmt.Errorf("wildcard chat ids are not allowed; add explicit numeric Telegram chat ids")
	}
	chatID, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid Telegram chat id %q: %w", value, err)
	}
	if chatID == 0 {
		return 0, fmt.Errorf("chat id 0 is not registerable; use clear to remove all registered chats")
	}
	return chatID, nil
}

func applyTelegramChatIDAction(ids []int64, action telegramChatIDAction, chatID int64) []int64 {
	ids = config.NormalizeChatIDs(ids)
	switch action {
	case telegramChatIDAdd:
		for _, id := range ids {
			if id == chatID {
				return ids
			}
		}
		return append(ids, chatID)
	case telegramChatIDDelete:
		out := ids[:0]
		for _, id := range ids {
			if id != chatID {
				out = append(out, id)
			}
		}
		return out
	case telegramChatIDClear:
		return nil
	default:
		return ids
	}
}

func runTelegramTokenCommand(args []string, in io.Reader, out io.Writer) error {
	var configPath string
	fs := flag.NewFlagSet("telegram token", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&configPath, "config", defaultConfigPath(), "path to Dexgram TOML config")
	if err := fs.Parse(args); err != nil {
		return err
	}
	action, err := parseTelegramTokenAction(fs.Args())
	if err != nil {
		return err
	}

	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("config file not found: %s; run %s onboard first", configPath, filepath.Base(os.Args[0]))
		}
		return fmt.Errorf("read config metadata: %w", err)
	}
	cfg, err := loadOnboardConfig(configPath)
	if err != nil {
		return err
	}

	switch action {
	case "update":
		reader := bufio.NewReader(in)
		token, err := promptRequired(reader, out, "Telegram bot token/id from @BotFather", cfg.Telegram.BotToken)
		if err != nil {
			return err
		}
		cfg.Telegram.BotToken = strings.TrimSpace(token)
	default:
		return fmt.Errorf("unknown token action %q", action)
	}
	applyOnboardDefaults(&cfg)
	if err := writeOnboardConfig(configPath, cfg); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "\nUpdated Telegram bot token in %s\n", configPath)
	return nil
}

func parseTelegramTokenAction(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: %s telegram token update", filepath.Base(os.Args[0]))
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	if len(args) != 1 || action != "update" {
		return "", fmt.Errorf("usage: %s telegram token update", filepath.Base(os.Args[0]))
	}
	return action, nil
}
