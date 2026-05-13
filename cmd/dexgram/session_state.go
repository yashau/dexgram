package main

import (
	"encoding/json"
	"strconv"

	"dexgram/internal/codex"
)

func (a *app) registerSession(key string, session *activeTurn) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active[key] != nil {
		return false
	}
	a.active[key] = session
	return true
}

func (a *app) activeSession(key string) *activeTurn {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.active[key]
}

func (a *app) addSessionTurn(key string, turn *telegramTurn) {
	a.mu.Lock()
	defer a.mu.Unlock()
	session := a.active[key]
	if session == nil {
		return
	}
	session.turns[turn.TurnID] = turn
	session.order = append(session.order, turn.TurnID)
}

func (a *app) takePendingTurnEvents(key, turnID string) []codex.Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	session := a.active[key]
	if session == nil || len(session.pendingEvents) == 0 {
		return nil
	}
	events := append([]codex.Event(nil), session.pendingEvents[turnID]...)
	delete(session.pendingEvents, turnID)
	return events
}

func (a *app) sessionTurn(key, turnID string) *telegramTurn {
	a.mu.Lock()
	defer a.mu.Unlock()
	session := a.active[key]
	if session == nil {
		return nil
	}
	return session.turns[turnID]
}

func (a *app) sessionTurnCount(key string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	session := a.active[key]
	if session == nil {
		return 0
	}
	return len(session.turns)
}

func (a *app) removeSessionTurn(key, turnID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	session := a.active[key]
	if session == nil {
		return
	}
	delete(session.turns, turnID)
	for i, id := range session.order {
		if id == turnID {
			session.order = append(session.order[:i], session.order[i+1:]...)
			break
		}
	}
}

func (a *app) currentTurnID(key string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	session := a.active[key]
	if session == nil {
		return ""
	}
	for _, id := range session.order {
		if session.turns[id] != nil {
			return id
		}
	}
	return ""
}

func (a *app) rememberTurnAction(key, turnID string) string {
	token := strconv.FormatInt(a.actionSeq.Add(1), 36)
	a.mu.Lock()
	a.actions[token] = turnAction{Key: key, TurnID: turnID}
	a.mu.Unlock()
	return token
}

func (a *app) turnAction(token string) (turnAction, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	action, ok := a.actions[token]
	return action, ok
}

func (a *app) forgetTurnAction(key, turnID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for token, action := range a.actions {
		if action.Key == key && action.TurnID == turnID {
			delete(a.actions, token)
		}
	}
}

func (a *app) release(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.active, key)
}

func (a *app) deferUnknownTurnEvent(key string, session *activeTurn, ev codex.Event) bool {
	turnID := eventTurnID(ev)
	if turnID == "" || a.sessionTurn(key, turnID) != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active[key] != session {
		return false
	}
	if session.pendingEvents == nil {
		session.pendingEvents = map[string][]codex.Event{}
	}
	session.pendingEvents[turnID] = append(session.pendingEvents[turnID], ev)
	return true
}

func eventTurnID(ev codex.Event) string {
	var payload struct {
		TurnID string `json:"turnId"`
		Turn   struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if json.Unmarshal(ev.Params, &payload) != nil {
		return ""
	}
	if payload.TurnID != "" {
		return payload.TurnID
	}
	return payload.Turn.ID
}
