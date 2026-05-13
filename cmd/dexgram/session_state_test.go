package main

import (
	"context"
	"testing"
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
	if got := app.currentTurnID("chat:topic"); got != "turn-1" {
		t.Fatalf("current turn = %q", got)
	}
	if got := app.sessionTurn("chat:topic", "turn-2"); got == nil || got.TurnID != "turn-2" {
		t.Fatalf("unexpected turn: %#v", got)
	}

	app.removeSessionTurn("chat:topic", "turn-1")
	if got := app.currentTurnID("chat:topic"); got != "turn-2" {
		t.Fatalf("current turn after removal = %q", got)
	}

	app.release("chat:topic")
	if got := app.activeSession("chat:topic"); got != nil {
		t.Fatalf("expected session to be released: %#v", got)
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

func newTestApp() *app {
	return &app{
		active:  map[string]*activeTurn{},
		actions: map[string]turnAction{},
	}
}
