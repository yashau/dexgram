package main

import (
	"context"
	"strings"
	"testing"

	"dexgram/internal/codex"
	"dexgram/internal/config"
	"dexgram/internal/state"
)

func TestSettingsTextAndReasoningMenuReflectStoredSettings(t *testing.T) {
	store, err := state.Open(t.TempDir() + "/dexgram.db")
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer closeTestStateStore(t, store)

	app := &app{store: store}
	if err := store.SetSetting(codexModelSettingKey, "gpt-5.4"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if err := store.SetSetting(codexReasoningSettingKey, "high"); err != nil {
		t.Fatalf("set effort: %v", err)
	}

	text := app.settingsText()
	if !strings.Contains(text, "Model: gpt-5.4") {
		t.Fatalf("settings text missing model: %q", text)
	}
	if !strings.Contains(text, "Reasoning effort: high") {
		t.Fatalf("settings text missing effort: %q", text)
	}

	title, markup := app.reasoningMenu()
	if title != "Codex reasoning effort\nCurrent: high" {
		t.Fatalf("reasoning title = %q", title)
	}
	if markup == nil || len(markup.InlineKeyboard) != 5 {
		t.Fatalf("unexpected reasoning markup: %#v", markup)
	}
	if got := markup.InlineKeyboard[2][1].Text; got != "* high" {
		t.Fatalf("selected high button = %q", got)
	}
	if got := markup.InlineKeyboard[0][0].Text; got != "Auto" {
		t.Fatalf("auto button = %q", got)
	}
}

func TestSettingsHelpersNormalizeLabelsAndValues(t *testing.T) {
	if got := settingLabel(" \t "); got != "Auto" {
		t.Fatalf("empty setting label = %q", got)
	}
	if got := selectedLabel("Auto", true); got != "* Auto" {
		t.Fatalf("selected label = %q", got)
	}
	if got := normalizeCollaborationMode(" PLAN-MODE "); got != "plan" {
		t.Fatalf("collaboration mode = %q", got)
	}
	if got := normalizeCollaborationMode("chat"); got != "" {
		t.Fatalf("unknown collaboration mode = %q", got)
	}
	if got := normalizeReasoningEffort(" Extra_High "); got != "xhigh" {
		t.Fatalf("reasoning alias = %q", got)
	}
	if got := normalizeReasoningEffort(" Medium "); got != "medium" {
		t.Fatalf("reasoning effort = %q", got)
	}

	long := strings.Repeat("a", 50)
	if got := shortButtonLabel(long); got != strings.Repeat("a", 45)+"..." {
		t.Fatalf("short button label = %q", got)
	}
	if !contains(reasoningEfforts, "medium") {
		t.Fatal("expected medium effort to be present")
	}
	if contains(reasoningEfforts, "extreme") {
		t.Fatal("unexpected extreme effort")
	}
	if !modelExists([]codex.ModelOption{{Model: "gpt-5.4"}, {ID: "fallback-id"}}, "fallback-id") {
		t.Fatal("expected model lookup to fall back to ID")
	}
}

func TestSettingsMarkupHasExpectedCallbacks(t *testing.T) {
	markup := settingsMarkup()
	if markup == nil || len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 2 {
		t.Fatalf("unexpected settings markup: %#v", markup)
	}
	if got := markup.InlineKeyboard[0][0].CallbackData; got != "settings:model" {
		t.Fatalf("model callback = %q", got)
	}
	if got := markup.InlineKeyboard[0][1].CallbackData; got != "settings:effort" {
		t.Fatalf("effort callback = %q", got)
	}
}

func TestTurnOptionsUsesStoredPlanSettings(t *testing.T) {
	store, err := state.Open(t.TempDir() + "/dexgram.db")
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer closeTestStateStore(t, store)

	if err := store.SetSetting(codexModelSettingKey, "gpt-5.4"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if err := store.SetSetting(codexReasoningSettingKey, "X-High"); err != nil {
		t.Fatalf("set effort: %v", err)
	}
	app := &app{
		cfg: &config.Config{Codex: config.CodexConfig{
			ApprovalPolicy: "on-request",
			Sandbox:        "workspace-write",
		}},
		store: store,
	}

	opts, err := app.turnOptions(context.Background(), nil, "plan-mode")
	if err != nil {
		t.Fatalf("turn options: %v", err)
	}
	if opts.ApprovalPolicy != "on-request" || opts.Sandbox != "workspace-write" {
		t.Fatalf("base options = %#v", opts)
	}
	if opts.CollaborationMode != "plan" || opts.Model != "gpt-5.4" || opts.ReasoningEffort != "xhigh" {
		t.Fatalf("stored plan options = %#v", opts)
	}
}
