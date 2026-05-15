package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"time"

	"dexgram/internal/config"

	"github.com/fsnotify/fsnotify"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type configFileState struct {
	modTime time.Time
	size    int64
}

type configReloadResult struct {
	reloaded        bool
	botTokenChanged bool
}

func statConfigFile(path string) (configFileState, error) {
	info, err := os.Stat(path)
	if err != nil {
		return configFileState{}, fmt.Errorf("read config metadata: %w", err)
	}
	return configFileState{modTime: info.ModTime(), size: info.Size()}, nil
}

func (s configFileState) equal(other configFileState) bool {
	return s.size == other.size && s.modTime.Equal(other.modTime)
}

func (a *app) watchConfigChanges(ctx context.Context, b *bot.Bot, restart chan<- struct{}) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("config file watcher: %v", err)
	} else if err := watcher.Add(filepath.Dir(a.configPath)); err != nil {
		log.Printf("config file watcher add %s: %v", filepath.Dir(a.configPath), err)
		_ = watcher.Close()
		watcher = nil
	}
	defer func() {
		if watcher != nil {
			_ = watcher.Close()
		}
	}()

	fallback := time.NewTicker(2 * time.Second)
	defer fallback.Stop()
	reload := func() bool {
		result := a.reloadConfigIfChanged(ctx, b)
		if result.botTokenChanged {
			select {
			case restart <- struct{}{}:
			default:
			}
			return true
		}
		return false
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-configWatcherEvents(watcher):
			if !ok {
				watcher = nil
				continue
			}
			if !configFileEventMatches(event, a.configPath) {
				continue
			}
			if reload() {
				return
			}
		case err, ok := <-configWatcherErrors(watcher):
			if !ok {
				watcher = nil
				continue
			}
			log.Printf("config file watcher: %v", err)
		case <-fallback.C:
			if reload() {
				return
			}
		}
	}
}

func (a *app) reloadConfigIfChanged(ctx context.Context, b *bot.Bot) configReloadResult {
	nextState, err := statConfigFile(a.configPath)
	if err != nil {
		log.Printf("check config reload: %v", err)
		return configReloadResult{}
	}
	if a.configState.equal(nextState) {
		return configReloadResult{}
	}

	next, err := config.Load(a.configPath)
	if err != nil {
		log.Printf("reload config: %v", err)
		return configReloadResult{}
	}

	oldChatIDs := append([]int64(nil), a.cfg.Telegram.ChatIDs...)
	oldBotToken := a.cfg.Telegram.BotToken
	a.cfg = next
	a.configState = nextState
	log.Printf("reloaded config from %s", a.configPath)

	if oldBotToken != next.Telegram.BotToken {
		log.Printf("telegram.bot_token changed; reloading Telegram bot")
		return configReloadResult{reloaded: true, botTokenChanged: true}
	}
	if !slices.Equal(config.NormalizeChatIDs(oldChatIDs), config.NormalizeChatIDs(next.Telegram.ChatIDs)) {
		if err := reconcileChangedCommands(ctx, b, oldChatIDs, next.Telegram.ChatIDs); err != nil {
			log.Printf("reconcile Telegram commands after config reload: %v", err)
		}
	}
	return configReloadResult{reloaded: true}
}

func reconcileChangedCommands(ctx context.Context, b *bot.Bot, oldChatIDs, newChatIDs []int64) error {
	clearChatIDs := append([]int64(nil), oldChatIDs...)
	clearChatIDs = append(clearChatIDs, newChatIDs...)
	var errs []error
	for _, scope := range telegramCommandClearScopes(clearChatIDs) {
		if _, err := b.DeleteMyCommands(ctx, &bot.DeleteMyCommandsParams{Scope: scope}); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	newChatIDs = config.NormalizeChatIDs(newChatIDs)
	if len(newChatIDs) == 0 {
		log.Printf("telegram slash commands cleared after config reload; no registered chats")
		return nil
	}
	for _, chatID := range newChatIDs {
		if _, err := b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
			Commands: telegramCommands(),
			Scope:    &models.BotCommandScopeChat{ChatID: chatID},
		}); err != nil {
			return fmt.Errorf("register chat commands for %d: %w", chatID, err)
		}
		if _, err := b.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
			ChatID:     chatID,
			MenuButton: &models.MenuButtonCommands{},
		}); err != nil {
			log.Printf("set Telegram menu button for chat_id=%d: %v", chatID, err)
		}
	}
	log.Printf("telegram slash commands reloaded for chat_ids=%v", newChatIDs)
	return nil
}

func configWatcherEvents(watcher *fsnotify.Watcher) <-chan fsnotify.Event {
	if watcher == nil {
		return nil
	}
	return watcher.Events
}

func configWatcherErrors(watcher *fsnotify.Watcher) <-chan error {
	if watcher == nil {
		return nil
	}
	return watcher.Errors
}

func configFileEventMatches(event fsnotify.Event, path string) bool {
	if event.Name == "" {
		return false
	}
	if filepath.Clean(event.Name) != filepath.Clean(path) {
		return false
	}
	return event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename)
}
