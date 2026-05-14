package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Telegram TelegramConfig `toml:"telegram"`
	Codex    CodexConfig    `toml:"codex"`
}

type TelegramConfig struct {
	BotToken               string  `toml:"bot_token"`
	ChatID                 int64   `toml:"chat_id"`
	ChatIDs                []int64 `toml:"chat_ids"`
	UploadFinalAnswerFiles bool    `toml:"upload_final_answer_files"`
}

type CodexConfig struct {
	CWD            string `toml:"cwd"`
	CLIPath        string `toml:"cli_path"`
	ApprovalPolicy string `toml:"approval_policy"`
	Sandbox        string `toml:"sandbox"`
}

func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, err
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	c.Telegram.BotToken = strings.TrimSpace(c.Telegram.BotToken)
	if len(c.Telegram.ChatIDs) == 0 && c.Telegram.ChatID != 0 {
		c.Telegram.ChatIDs = []int64{c.Telegram.ChatID}
	}
	c.Telegram.ChatIDs = NormalizeChatIDs(c.Telegram.ChatIDs)
	c.Codex.CWD = strings.TrimSpace(c.Codex.CWD)
	c.Codex.CLIPath = strings.TrimSpace(c.Codex.CLIPath)
	c.Codex.ApprovalPolicy = strings.TrimSpace(c.Codex.ApprovalPolicy)
	c.Codex.Sandbox = strings.TrimSpace(c.Codex.Sandbox)
	if c.Codex.ApprovalPolicy == "" {
		c.Codex.ApprovalPolicy = "never"
	}
	if c.Codex.Sandbox == "" {
		c.Codex.Sandbox = "danger-full-access"
	}
}

func NormalizeChatIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := map[int64]bool{}
	normalized := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		normalized = append(normalized, id)
	}
	return normalized
}

func (c *Config) validate() error {
	if c.Telegram.BotToken == "" {
		return errors.New("telegram.bot_token is required")
	}
	return nil
}
