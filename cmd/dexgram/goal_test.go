package main

import (
	"context"
	"testing"
	"time"

	"dexgram/internal/codex"

	"github.com/go-telegram/bot/models"
)

func TestHandleUpdateGoalWithoutArgsShowsCurrentGoal(t *testing.T) {
	b, api := newTelegramTestBot(t)
	testApp := newHandlerTestApp(t, []int64{123})

	oldGetGoal := getTopicGoalFunc
	getTopicGoalFunc = func(_ *app, _ context.Context, chatID int64, messageThreadID int) (*codex.ThreadGoal, error) {
		if chatID != 123 || messageThreadID != 7 {
			t.Fatalf("get goal target = %d:%d", chatID, messageThreadID)
		}
		return &codex.ThreadGoal{
			Objective: "Ship the usage command",
			Status:    "active",
		}, nil
	}
	defer func() {
		getTopicGoalFunc = oldGetGoal
	}()

	testApp.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		ID:              5,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "/goal",
	}})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !api.bodyContains("sendMessage", "Ship the usage command") {
		time.Sleep(10 * time.Millisecond)
	}
	if !api.bodyContains("sendMessage", "Status: active") {
		t.Fatalf("goal status was not sent: %#v", api.calls)
	}
	if api.bodyContains("sendMessage", "How should Dexgram use this message?") {
		t.Fatalf("/goal fell through to Codex prompt handling: %#v", api.calls)
	}
}

func TestHandleUpdateGoalPauseStoresAndClearsGoal(t *testing.T) {
	b, api := newTelegramTestBot(t)
	testApp := newHandlerTestApp(t, []int64{123})

	oldSetGoal := setTopicGoalFunc
	oldPauseGoal := pauseTopicGoalFunc
	setTopicGoalFunc = func(_ *app, _ context.Context, _ int64, _ int, objective string) error {
		t.Fatalf("pause should not set objective %q", objective)
		return nil
	}
	pauseCalled := false
	pauseTopicGoalFunc = func(_ *app, _ context.Context, chatID int64, messageThreadID int) (string, error) {
		pauseCalled = true
		if chatID != 123 || messageThreadID != 7 {
			t.Fatalf("pause goal target = %d:%d", chatID, messageThreadID)
		}
		return "Finish Dexgram goals", nil
	}
	defer func() {
		setTopicGoalFunc = oldSetGoal
		pauseTopicGoalFunc = oldPauseGoal
	}()

	testApp.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		ID:              5,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "/goal pause",
	}})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !api.bodyContains("sendMessage", "Codex goal paused:") {
		time.Sleep(10 * time.Millisecond)
	}
	if !pauseCalled {
		t.Fatal("expected pause goal path to be called")
	}
	if !api.bodyContains("sendMessage", "Finish Dexgram goals") {
		t.Fatalf("goal pause confirmation was not sent: %#v", api.calls)
	}
	if api.bodyContains("sendMessage", "How should Dexgram use this message?") {
		t.Fatalf("/goal pause fell through to Codex prompt handling: %#v", api.calls)
	}
}

func TestHandleUpdateGoalResumeRestoresPausedGoal(t *testing.T) {
	b, api := newTelegramTestBot(t)
	testApp := newHandlerTestApp(t, []int64{123})

	oldSetGoal := setTopicGoalFunc
	oldResumeGoal := resumeTopicGoalFunc
	setTopicGoalFunc = func(_ *app, _ context.Context, _ int64, _ int, objective string) error {
		t.Fatalf("resume should not set a literal objective %q", objective)
		return nil
	}
	resumeCalled := false
	resumeTopicGoalFunc = func(_ *app, _ context.Context, chatID int64, messageThreadID int) (string, error) {
		resumeCalled = true
		if chatID != 123 || messageThreadID != 7 {
			t.Fatalf("resume goal target = %d:%d", chatID, messageThreadID)
		}
		return "Finish Dexgram goals", nil
	}
	defer func() {
		setTopicGoalFunc = oldSetGoal
		resumeTopicGoalFunc = oldResumeGoal
	}()

	testApp.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		ID:              5,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "/goal resume",
	}})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !api.bodyContains("sendMessage", "Codex goal resumed:") {
		time.Sleep(10 * time.Millisecond)
	}
	if !resumeCalled {
		t.Fatal("expected resume goal path to be called")
	}
	if !api.bodyContains("sendMessage", "Finish Dexgram goals") {
		t.Fatalf("goal resume confirmation was not sent: %#v", api.calls)
	}
	if api.bodyContains("sendMessage", "How should Dexgram use this message?") {
		t.Fatalf("/goal resume fell through to Codex prompt handling: %#v", api.calls)
	}
}

func TestPausedGoalObjectiveRequiresLivePausedStatus(t *testing.T) {
	if got := pausedGoalObjective(&codex.ThreadGoal{Objective: "Resume me", Status: " paused "}); got != "Resume me" {
		t.Fatalf("pausedGoalObjective = %q", got)
	}
	for _, goal := range []*codex.ThreadGoal{
		nil,
		{Objective: "Active", Status: "active"},
		{Objective: "   ", Status: "paused"},
	} {
		if got := pausedGoalObjective(goal); got != "" {
			t.Fatalf("pausedGoalObjective(%#v) = %q", goal, got)
		}
	}
}

func TestHandleUpdateGoalClearPhraseSetsObjective(t *testing.T) {
	b, api := newTelegramTestBot(t)
	testApp := newHandlerTestApp(t, []int64{123})

	oldSetGoal := setTopicGoalFunc
	oldClearGoal := clearTopicGoalFunc
	setCalled := false
	setTopicGoalFunc = func(_ *app, _ context.Context, chatID int64, messageThreadID int, objective string) error {
		setCalled = true
		if chatID != 123 || messageThreadID != 7 || objective != "clear database migration" {
			t.Fatalf("set goal args = %d:%d %q", chatID, messageThreadID, objective)
		}
		return nil
	}
	clearTopicGoalFunc = func(_ *app, _ context.Context, _ int64, _ int) error {
		t.Fatal("multi-word goal should not clear current goal")
		return nil
	}
	defer func() {
		setTopicGoalFunc = oldSetGoal
		clearTopicGoalFunc = oldClearGoal
	}()

	testApp.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		ID:              5,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "/goal clear database migration",
	}})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !api.bodyContains("sendMessage", "Codex goal set:") {
		time.Sleep(10 * time.Millisecond)
	}
	if !setCalled {
		t.Fatal("expected set goal path to be called")
	}
	if !api.bodyContains("sendMessage", "clear database migration") {
		t.Fatalf("goal set confirmation was not sent: %#v", api.calls)
	}
}
