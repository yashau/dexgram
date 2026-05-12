package main

import "strconv"

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
