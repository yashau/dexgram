package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const pendingUpdateNoticeSettingKey = "dexgram.update.pending_notice"

type pendingUpdateNotice struct {
	ChatID          int64  `json:"chat_id"`
	MessageThreadID int    `json:"message_thread_id"`
	FromVersion     string `json:"from_version"`
	TargetVersion   string `json:"target_version"`
	StartedAt       string `json:"started_at"`
}

func (a *app) handleUpdateCommand(ctx context.Context, b *bot.Bot, msg *models.Message) {
	checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	latest, cmp, err := latestUpdateComparison(checkCtx, appVersion)
	if err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Could not check for a Dexgram update:\n\n" + err.Error(),
		})
		return
	}
	targetVersion := cleanVersion(latest)
	if cmp >= 0 {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            fmt.Sprintf("Dexgram is already up to date (%s).", appVersion),
		})
		return
	}

	notice := pendingUpdateNotice{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		FromVersion:     appVersion,
		TargetVersion:   targetVersion,
		StartedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if err := a.savePendingUpdateNotice(notice); err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Could not save Dexgram update state:\n\n" + err.Error(),
		})
		return
	}

	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            pendingUpdateRestartMessage(notice),
	}); err != nil {
		_ = a.clearPendingUpdateNotice()
		log.Printf("send update restart notice: %v", err)
		return
	}

	if err := startUpdateProcess(a.logPath); err != nil {
		_ = a.clearPendingUpdateNotice()
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Could not start the Dexgram updater:\n\n" + err.Error(),
		})
	}
}

func (a *app) sendPendingUpdateNotice(ctx context.Context, b *bot.Bot) error {
	notice, ok, err := a.loadPendingUpdateNotice()
	if err != nil {
		_ = a.clearPendingUpdateNotice()
		return err
	}
	if !ok {
		return nil
	}
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          notice.ChatID,
		MessageThreadID: notice.MessageThreadID,
		Text:            pendingUpdateCompleteMessage(notice, appVersion),
	}); err != nil {
		return err
	}
	return a.clearPendingUpdateNotice()
}

func (a *app) savePendingUpdateNotice(notice pendingUpdateNotice) error {
	data, err := json.Marshal(notice)
	if err != nil {
		return err
	}
	return a.store.SetSetting(pendingUpdateNoticeSettingKey, string(data))
}

func (a *app) loadPendingUpdateNotice() (pendingUpdateNotice, bool, error) {
	raw, err := a.store.GetSetting(pendingUpdateNoticeSettingKey)
	if err != nil {
		return pendingUpdateNotice{}, false, err
	}
	if strings.TrimSpace(raw) == "" {
		return pendingUpdateNotice{}, false, nil
	}
	var notice pendingUpdateNotice
	if err := json.Unmarshal([]byte(raw), &notice); err != nil {
		return pendingUpdateNotice{}, false, fmt.Errorf("parse pending update notice: %w", err)
	}
	if notice.ChatID == 0 {
		return pendingUpdateNotice{}, false, fmt.Errorf("pending update notice has no chat id")
	}
	return notice, true, nil
}

func (a *app) clearPendingUpdateNotice() error {
	return a.store.SetSetting(pendingUpdateNoticeSettingKey, "")
}

func pendingUpdateRestartMessage(notice pendingUpdateNotice) string {
	return fmt.Sprintf(
		"Restarting Dexgram for update (%s -> %s). I will message this topic when I am back.",
		notice.FromVersion,
		notice.TargetVersion,
	)
}

func pendingUpdateCompleteMessage(notice pendingUpdateNotice, currentVersion string) string {
	if notice.TargetVersion != "" && notice.TargetVersion != currentVersion {
		return fmt.Sprintf("Dexgram is back after updating to %s. Current version: %s.", notice.TargetVersion, currentVersion)
	}
	return fmt.Sprintf("Dexgram is back after updating to %s.", currentVersion)
}

func cleanVersion(version string) string {
	return strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(version), "v"), "V")
}
