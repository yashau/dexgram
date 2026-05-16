package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"dexgram/internal/codex"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var readUsageSnapshotFunc = (*app).readUsageSnapshot

func (a *app) handleUsageCommand(ctx context.Context, b *bot.Bot, msg *models.Message) {
	snapshot, err := readUsageSnapshotFunc(a, ctx, msg.Chat.ID, msg.MessageThreadID)
	if err != nil {
		log.Printf("read Codex usage failed: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Dexgram could not read Codex usage:\n\n" + err.Error(),
		})
		return
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:              msg.Chat.ID,
		MessageThreadID:     msg.MessageThreadID,
		Text:                formatUsageSnapshot(snapshot, time.Now()),
		DisableNotification: true,
	})
}

func (a *app) readUsageSnapshot(ctx context.Context, chatID int64, messageThreadID int) (codex.RateLimitSnapshot, error) {
	key := fmt.Sprintf("%d:%d", chatID, messageThreadID)
	if session := a.activeSession(key); session != nil {
		return readCodexUsage(ctx, session.client)
	}

	cliPath := ""
	if a.cfg != nil {
		cliPath = a.cfg.Codex.CLIPath
	}
	c, err := codex.StartStdioWithOptions(ctx, codex.StartOptions{CLIPath: cliPath})
	if err != nil {
		return codex.RateLimitSnapshot{}, err
	}
	done := make(chan struct{})
	defer close(done)
	defer func() {
		_ = c.Close()
	}()
	errs := c.Errors()
	go func() {
		for {
			select {
			case err, ok := <-errs:
				if !ok {
					return
				}
				log.Printf("codex app-server: %v", err)
			case <-done:
				return
			}
		}
	}()
	return readCodexUsage(ctx, c)
}

func readCodexUsage(ctx context.Context, c *codex.Client) (codex.RateLimitSnapshot, error) {
	var out codex.AccountRateLimitsResponse
	if err := c.Call(ctx, "account/rateLimits/read", nil, &out); err != nil {
		return codex.RateLimitSnapshot{}, err
	}
	snapshot, ok := selectUsageSnapshot(out)
	if !ok {
		return codex.RateLimitSnapshot{}, fmt.Errorf("Codex did not return usage limits")
	}
	return snapshot, nil
}

func selectUsageSnapshot(resp codex.AccountRateLimitsResponse) (codex.RateLimitSnapshot, bool) {
	if resp.RateLimitsByLimitID != nil {
		if snapshot := resp.RateLimitsByLimitID["codex"]; snapshot != nil && !emptyUsageSnapshot(*snapshot) {
			return *snapshot, true
		}
		for _, snapshot := range resp.RateLimitsByLimitID {
			if snapshot == nil || emptyUsageSnapshot(*snapshot) {
				continue
			}
			if strings.Contains(strings.ToLower(snapshot.LimitID), "codex") ||
				strings.Contains(strings.ToLower(snapshot.LimitName), "codex") {
				return *snapshot, true
			}
		}
	}
	if !emptyUsageSnapshot(resp.RateLimits) {
		return resp.RateLimits, true
	}
	return codex.RateLimitSnapshot{}, false
}

func emptyUsageSnapshot(snapshot codex.RateLimitSnapshot) bool {
	return snapshot.Primary == nil && snapshot.Secondary == nil && snapshot.LimitID == "" && snapshot.LimitName == ""
}

func formatUsageSnapshot(snapshot codex.RateLimitSnapshot, now time.Time) string {
	title := "Codex usage"
	if label := strings.TrimSpace(snapshot.LimitName); label != "" {
		title += " (" + label + ")"
	} else if label := strings.TrimSpace(snapshot.LimitID); label != "" {
		title += " (" + label + ")"
	}
	lines := []string{title}
	fiveHourLine := ""
	weeklyLine := ""
	otherLines := []string{}
	for _, window := range sortedUsageWindows(snapshot) {
		line := formatUsageWindow(window.name, window.window, now)
		switch window.name {
		case "5-hour":
			fiveHourLine = line
		case "Weekly":
			weeklyLine = line
		default:
			otherLines = append(otherLines, line)
		}
	}
	if fiveHourLine == "" {
		fiveHourLine = "5-hour: not reported"
	}
	if weeklyLine == "" {
		weeklyLine = "Weekly: not reported"
	}
	lines = append(lines, fiveHourLine, weeklyLine)
	lines = append(lines, otherLines...)
	if reached := strings.TrimSpace(snapshot.RateLimitReachedType); reached != "" {
		lines = append(lines, "Limit reached: "+reached)
	}
	return strings.Join(lines, "\n")
}

type namedUsageWindow struct {
	name   string
	window *codex.RateLimitWindow
}

func sortedUsageWindows(snapshot codex.RateLimitSnapshot) []namedUsageWindow {
	windows := []namedUsageWindow{}
	if snapshot.Primary != nil {
		windows = append(windows, namedUsageWindow{name: usageWindowName(snapshot.Primary), window: snapshot.Primary})
	}
	if snapshot.Secondary != nil {
		windows = append(windows, namedUsageWindow{name: usageWindowName(snapshot.Secondary), window: snapshot.Secondary})
	}
	sort.SliceStable(windows, func(i, j int) bool {
		return usageWindowDuration(windows[i].window) < usageWindowDuration(windows[j].window)
	})
	return windows
}

func formatUsageWindow(name string, window *codex.RateLimitWindow, now time.Time) string {
	if window == nil {
		return name + ": not reported"
	}
	line := fmt.Sprintf("%s: %s used", name, formatPercent(window.UsedPercent))
	if reset := formatUsageReset(window.ResetsAt, now); reset != "" {
		line += ", " + reset
	}
	return line
}

func usageWindowName(window *codex.RateLimitWindow) string {
	mins := usageWindowDuration(window)
	switch {
	case mins > 0 && math.Abs(mins-300) < 0.5:
		return "5-hour"
	case mins > 0 && math.Abs(mins-10080) < 0.5:
		return "Weekly"
	case mins > 0 && mins < 1440:
		return fmt.Sprintf("%s-hour", formatNumber(mins/60))
	case mins > 0:
		return fmt.Sprintf("%s-day", formatNumber(mins/1440))
	default:
		return "Usage"
	}
}

func usageWindowDuration(window *codex.RateLimitWindow) float64 {
	if window == nil || window.WindowDurationMins == nil {
		return math.MaxFloat64
	}
	return *window.WindowDurationMins
}

func formatPercent(value float64) string {
	return formatNumber(value) + "%"
}

func formatNumber(value float64) string {
	if math.Abs(value-math.Round(value)) < 0.05 {
		return fmt.Sprintf("%.0f", math.Round(value))
	}
	return fmt.Sprintf("%.1f", value)
}

func formatUsageReset(value *float64, now time.Time) string {
	if value == nil || *value <= 0 {
		return ""
	}
	seconds := *value
	if seconds > 1_000_000_000_000 {
		seconds /= 1000
	}
	resetAt := time.Unix(int64(seconds), 0).Local()
	until := resetAt.Sub(now)
	if until <= 0 {
		return "reset is available"
	}
	return "resets in " + compactDuration(until) + " (" + resetAt.Format("2006-01-02 15:04") + ")"
}

func compactDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	totalMinutes := int(math.Ceil(d.Minutes()))
	days := totalMinutes / (24 * 60)
	totalMinutes %= 24 * 60
	hours := totalMinutes / 60
	minutes := totalMinutes % 60
	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if days == 0 && minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if len(parts) == 0 {
		return "<1m"
	}
	return strings.Join(parts, " ")
}
