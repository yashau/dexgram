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
	BotToken string `toml:"bot_token"`
	ChatID   int64  `toml:"chat_id"`
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

func (c *Config) validate() error {
	if c.Telegram.BotToken == "" {
		return errors.New("telegram.bot_token is required")
	}
	return nil
}
