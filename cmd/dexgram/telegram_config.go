package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dexgram/internal/config"
)

func runTelegramCommand(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printTelegramHelp(os.Stdout, filepath.Base(os.Args[0]))
		return nil
	}

	switch args[0] {
	case "chatid", "chat-id", "id":
		return runTelegramChatIDCommand(args[1:], os.Stdout)
	default:
		return fmt.Errorf("unknown telegram command %q; run %s telegram --help", args[0], filepath.Base(os.Args[0]))
	}
}

func printTelegramHelp(w io.Writer, exe string) {
	_, _ = fmt.Fprintf(w, `Dexgram Telegram Commands

Usage

  %[1]s telegram chatid add <chat_id>
  %[1]s telegram chatid del <chat_id>
  %[1]s telegram chatid clear
  %[1]s tg id add <chat_id>

Options

  -config path
      Path to Dexgram TOML config. Defaults to .\dexgram.toml if present,
      otherwise %%APPDATA%%\Dexgram\dexgram.toml.

Examples

  %[1]s telegram chatid add 123456789
  %[1]s telegram chatid add -1001234567890
  %[1]s telegram chatid del 123456789
  %[1]s telegram chatid clear

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
	action, chatID, err := parseTelegramChatIDAction(fs.Args())
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

func parseTelegramChatIDAction(args []string) (telegramChatIDAction, int64, error) {
	if len(args) == 0 {
		return "", 0, fmt.Errorf("usage: %s telegram chatid <add|del|clear> [chat_id]", filepath.Base(os.Args[0]))
	}
	action := strings.ToLower(strings.TrimSpace(args[0]))
	switch action {
	case "add":
		if len(args) != 2 {
			return "", 0, fmt.Errorf("missing chat id; retry as: %s telegram chatid add <chat_id>", filepath.Base(os.Args[0]))
		}
		chatID, err := parseTelegramChatID(args[1])
		return telegramChatIDAdd, chatID, err
	case "del", "delete", "rm", "remove":
		if len(args) != 2 {
			return "", 0, fmt.Errorf("missing chat id; retry as: %s telegram chatid del <chat_id>", filepath.Base(os.Args[0]))
		}
		chatID, err := parseTelegramChatID(args[1])
		return telegramChatIDDelete, chatID, err
	case "clear":
		if len(args) != 1 {
			return "", 0, fmt.Errorf("usage: %s telegram chatid clear", filepath.Base(os.Args[0]))
		}
		return telegramChatIDClear, 0, nil
	default:
		return "", 0, fmt.Errorf("unknown chatid action %q; use add, del, or clear", args[0])
	}
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
