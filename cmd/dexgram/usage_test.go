package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"dexgram/internal/codex"

	"github.com/go-telegram/bot/models"
)

func TestSelectUsageSnapshotPrefersCodexLimitBucket(t *testing.T) {
	fiveHours := 300.0
	weekly := 10080.0
	resp := codex.AccountRateLimitsResponse{
		RateLimits: codex.RateLimitSnapshot{LimitID: "fallback"},
		RateLimitsByLimitID: map[string]*codex.RateLimitSnapshot{
			"other": {LimitID: "other"},
			"codex": {
				LimitID:   "codex",
				LimitName: "Codex",
				Primary:   &codex.RateLimitWindow{UsedPercent: 12.5, WindowDurationMins: &fiveHours},
				Secondary: &codex.RateLimitWindow{UsedPercent: 40, WindowDurationMins: &weekly},
			},
		},
	}

	got, ok := selectUsageSnapshot(resp)
	if !ok {
		t.Fatal("expected usage snapshot")
	}
	if got.LimitID != "codex" || got.Primary == nil || got.Secondary == nil {
		t.Fatalf("selected snapshot = %#v", got)
	}
}

func TestFormatUsageSnapshotShowsFiveHourAndWeeklyWindows(t *testing.T) {
	fiveHours := 300.0
	weekly := 10080.0
	reset5h := float64(time.Date(2026, 5, 17, 12, 30, 0, 0, time.Local).Unix())
	resetWeekly := float64(time.Date(2026, 5, 20, 9, 0, 0, 0, time.Local).Unix())
	snapshot := codex.RateLimitSnapshot{
		LimitName: "Codex",
		Primary:   &codex.RateLimitWindow{UsedPercent: 12.5, WindowDurationMins: &fiveHours, ResetsAt: &reset5h},
		Secondary: &codex.RateLimitWindow{UsedPercent: 40, WindowDurationMins: &weekly, ResetsAt: &resetWeekly},
	}
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.Local)

	got := formatUsageSnapshot(snapshot, now)
	for _, want := range []string{
		"Codex usage (Codex)",
		"5-hour: 12.5% used, resets in 2h 30m",
		"Weekly: 40% used, resets in 2d 23h",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("usage text missing %q: %q", want, got)
		}
	}
}

func TestHandleUpdateRoutesUsageCommand(t *testing.T) {
	b, api := newTelegramTestBot(t)
	testApp := newHandlerTestApp(t, []int64{123})

	fiveHours := 300.0
	oldReadUsage := readUsageSnapshotFunc
	readUsageSnapshotFunc = func(_ *app, _ context.Context, chatID int64, messageThreadID int) (codex.RateLimitSnapshot, error) {
		if chatID != 123 || messageThreadID != 7 {
			t.Fatalf("usage command target = %d:%d", chatID, messageThreadID)
		}
		return codex.RateLimitSnapshot{
			LimitName: "Codex",
			Primary:   &codex.RateLimitWindow{UsedPercent: 9, WindowDurationMins: &fiveHours},
		}, nil
	}
	defer func() {
		readUsageSnapshotFunc = oldReadUsage
	}()

	testApp.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		ID:              5,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "/usage",
	}})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !api.bodyContains("sendMessage", "5-hour") {
		time.Sleep(10 * time.Millisecond)
	}
	if !api.bodyContains("sendMessage", "5-hour") {
		t.Fatalf("usage message was not sent: %#v", api.calls)
	}
	if api.bodyContains("sendMessage", "How should Dexgram use this message?") {
		t.Fatalf("/usage fell through to Codex prompt handling: %#v", api.calls)
	}
}
