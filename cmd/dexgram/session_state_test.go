package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"dexgram/internal/codex"

	"github.com/go-telegram/bot"
)

func TestSessionStateTracksTurnsInOrder(t *testing.T) {
	app := newTestApp()
	session := &activeTurn{
		ctx:   context.Background(),
		turns: map[string]*telegramTurn{},
	}
	if !app.registerSession("chat:topic", session) {
		t.Fatal("expected first register to succeed")
	}
	if app.registerSession("chat:topic", &activeTurn{}) {
		t.Fatal("expected duplicate register to fail")
	}

	app.addSessionTurn("chat:topic", &telegramTurn{TurnID: "turn-1"})
	app.addSessionTurn("chat:topic", &telegramTurn{TurnID: "turn-2"})

	if got := app.sessionTurnCount("chat:topic"); got != 2 {
		t.Fatalf("turn count = %d", got)
	}
	if got := app.currentTurnID("chat:topic"); got != "turn-2" {
		t.Fatalf("current turn = %q", got)
	}
	if got := app.sessionTurn("chat:topic", "turn-2"); got == nil || got.TurnID != "turn-2" {
		t.Fatalf("unexpected turn: %#v", got)
	}

	app.removeSessionTurn("chat:topic", "turn-2")
	if got := app.currentTurnID("chat:topic"); got != "turn-1" {
		t.Fatalf("current turn after removal = %q", got)
	}

	app.release("chat:topic")
	if got := app.activeSession("chat:topic"); got != nil {
		t.Fatalf("expected session to be released: %#v", got)
	}
}

func TestSessionStatePromotesLocalQueuedTurns(t *testing.T) {
	app := newTestApp()
	session := &activeTurn{
		ctx:   context.Background(),
		turns: map[string]*telegramTurn{},
	}
	if !app.registerSession("chat:topic", session) {
		t.Fatal("expected session register")
	}

	app.addSessionTurn("chat:topic", &telegramTurn{TurnID: "active-turn"})
	localID := app.nextQueuedTurnID()
	app.addSessionTurn("chat:topic", &telegramTurn{TurnID: localID, Queued: true, Text: "later"})

	if got := app.currentTurnID("chat:topic"); got != "active-turn" {
		t.Fatalf("current turn = %q", got)
	}
	queued := app.nextQueuedSessionTurn("chat:topic")
	if queued == nil || queued.TurnID != localID {
		t.Fatalf("next queued turn = %#v", queued)
	}

	promoted := app.promoteSessionTurn("chat:topic", localID, "codex-turn")
	if promoted == nil || promoted.Queued || promoted.TurnID != "codex-turn" {
		t.Fatalf("promoted turn = %#v", promoted)
	}
	if got := app.sessionTurn("chat:topic", localID); got != nil {
		t.Fatalf("local turn id should be gone: %#v", got)
	}
	if got := app.sessionTurn("chat:topic", "codex-turn"); got == nil || got.Text != "later" {
		t.Fatalf("codex turn missing: %#v", got)
	}

	app.removeSessionTurn("chat:topic", "active-turn")
	if got := app.currentTurnID("chat:topic"); got != "codex-turn" {
		t.Fatalf("current turn after promotion = %q", got)
	}
}

func TestTurnActionsAreRememberedAndForgotten(t *testing.T) {
	app := newTestApp()

	token1 := app.rememberTurnAction("key", "turn-1")
	token2 := app.rememberTurnAction("key", "turn-2")
	if token1 == "" || token2 == "" || token1 == token2 {
		t.Fatalf("unexpected tokens: %q %q", token1, token2)
	}

	action, ok := app.turnAction(token1)
	if !ok || action.Key != "key" || action.TurnID != "turn-1" {
		t.Fatalf("unexpected action: %#v ok=%v", action, ok)
	}

	app.forgetTurnAction("key", "turn-1")
	if _, ok := app.turnAction(token1); ok {
		t.Fatal("expected first action to be forgotten")
	}
	if _, ok := app.turnAction(token2); !ok {
		t.Fatal("expected unrelated action to remain")
	}
}

func TestUnknownTurnEventsAreDeferredAndReplayed(t *testing.T) {
	app := newTestApp()
	session := &activeTurn{
		ctx:           context.Background(),
		turns:         map[string]*telegramTurn{},
		pendingEvents: map[string][]codex.Event{},
	}
	if !app.registerSession("chat:topic", session) {
		t.Fatal("expected session register")
	}

	params, err := json.Marshal(map[string]any{
		"turnId": "turn-1",
		"item":   map[string]any{"id": "item-1", "type": "plan", "text": "working"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ev := codex.Event{Method: "item/completed", Params: params}
	if !app.deferUnknownTurnEvent("chat:topic", session, ev) {
		t.Fatal("expected event to be deferred")
	}

	app.addSessionTurn("chat:topic", &telegramTurn{TurnID: "turn-1"})
	events := app.takePendingTurnEvents("chat:topic", "turn-1")
	if len(events) != 1 || events[0].Method != ev.Method {
		t.Fatalf("unexpected deferred events: %#v", events)
	}
	if events := app.takePendingTurnEvents("chat:topic", "turn-1"); len(events) != 0 {
		t.Fatalf("expected deferred events to be consumed: %#v", events)
	}
}

func TestUnknownTurnStartedDefersOnlyWhileStartingTelegramTurn(t *testing.T) {
	app := newTestApp()
	session := &activeTurn{
		ctx:           context.Background(),
		turns:         map[string]*telegramTurn{},
		pendingEvents: map[string][]codex.Event{},
	}
	if !app.registerSession("chat:topic", session) {
		t.Fatal("expected session register")
	}
	params, err := json.Marshal(map[string]any{
		"threadId": "thread-1",
		"turn":     map[string]any{"id": "turn-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ev := codex.Event{Method: "turn/started", Params: params}
	if app.deferUnknownTurnEvent("chat:topic", session, ev) {
		t.Fatal("autonomous turn/started should not be deferred")
	}

	app.beginSessionStartingTurn("chat:topic", session)
	if !app.deferUnknownTurnEvent("chat:topic", session, ev) {
		t.Fatal("telegram turn/started should be deferred while turn/start is in flight")
	}
	app.endSessionStartingTurn("chat:topic", session)
	events := app.takePendingTurnEvents("chat:topic", "turn-1")
	if len(events) != 1 || events[0].Method != "turn/started" {
		t.Fatalf("unexpected deferred turn/started events: %#v", events)
	}
	if app.sessionStartingTurn("chat:topic", session) {
		t.Fatal("starting turn counter should be cleared")
	}
}

func TestTypingActionReservationIsGloballyThrottled(t *testing.T) {
	app := newTestApp()
	if !app.reserveTypingAction(123, 456) {
		t.Fatal("expected first typing action to be reserved")
	}
	if app.reserveTypingAction(123, 456) {
		t.Fatal("expected immediate second typing action to be throttled")
	}

	app.mu.Lock()
	app.lastTypingAt = time.Now().Add(-typingGlobalMinInterval)
	app.mu.Unlock()
	if !app.reserveTypingAction(123, 456) {
		t.Fatal("expected typing action after interval to be reserved")
	}
}

func TestTypingActionSuppressionIsPerThread(t *testing.T) {
	app := newTestApp()
	app.suppressTypingActions(123, 456, time.Minute)

	if app.reserveTypingAction(123, 456) {
		t.Fatal("expected suppressed typing action to be skipped")
	}
	if !app.reserveTypingAction(123, 789) {
		t.Fatal("expected different thread to be allowed")
	}
}

func TestTypingRateLimitDoesNotBackoffTelegramQueue(t *testing.T) {
	oldQueue := defaultTelegramQueue
	defaultTelegramQueue = newTelegramQueue(time.Hour)
	defer func() {
		defaultTelegramQueue = oldQueue
	}()

	retryAfter, ok := logTelegramTypingPressure(123, 456, &bot.TooManyRequestsError{
		Message:    "too many requests",
		RetryAfter: 1,
	})
	if !ok {
		t.Fatal("expected typing rate limit to be recognized")
	}
	if retryAfter != time.Second {
		t.Fatalf("retryAfter = %s, want 1s", retryAfter)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := waitTelegramQueue(ctx, "normal send after typing pressure", 123, 456); err != nil {
		t.Fatalf("typing pressure backed off the main telegram queue: %v", err)
	}
}

func newTestApp() *app {
	return &app{
		active:                map[string]*activeTurn{},
		actions:               map[string]turnAction{},
		typingSuppressedUntil: map[string]time.Time{},
	}
}
