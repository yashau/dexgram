package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dexgram/internal/codex"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type permissionRequestParams struct {
	ThreadID    string          `json:"threadId"`
	TurnID      string          `json:"turnId"`
	ItemID      string          `json:"itemId"`
	CWD         string          `json:"cwd"`
	Reason      *string         `json:"reason"`
	Permissions json.RawMessage `json:"permissions"`
}

type userInputRequestParams struct {
	ThreadID  string          `json:"threadId"`
	TurnID    string          `json:"turnId"`
	ItemID    string          `json:"itemId"`
	Questions []inputQuestion `json:"questions"`
}

type inputQuestion struct {
	ID       string        `json:"id"`
	Header   string        `json:"header"`
	Question string        `json:"question"`
	IsOther  bool          `json:"isOther"`
	IsSecret bool          `json:"isSecret"`
	Options  []inputOption `json:"options"`
}

type inputOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

func (a *app) requestApproval(ctx context.Context, chatID int64, messageThreadID int, req codex.ServerRequest) (any, error) {
	switch req.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		return a.requestCommandOrFileApproval(ctx, chatID, messageThreadID, req)
	case "item/permissions/requestApproval":
		return a.requestPermissionApproval(ctx, chatID, messageThreadID, req)
	case "item/tool/requestUserInput":
		return a.requestUserInput(ctx, chatID, messageThreadID, req)
	default:
		return nil, fmt.Errorf("unsupported approval request: %s", req.Method)
	}
}

func (a *app) requestCommandOrFileApproval(ctx context.Context, chatID int64, messageThreadID int, req codex.ServerRequest) (any, error) {
	var params approvalRequestParams
	_ = json.Unmarshal(req.Params, &params)
	token := strconv.FormatInt(a.approvalSeq.Add(1), 36)
	pending := &pendingApproval{ch: make(chan approvalDecision, 1)}
	a.mu.Lock()
	a.approvals[token] = pending
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.approvals, token)
		a.mu.Unlock()
	}()

	text := approvalText(req.Method, params)
	buttons := [][]models.InlineKeyboardButton{
		{
			{Text: "Approve once", CallbackData: "ap:" + token + ":a"},
			{Text: "Approve session", CallbackData: "ap:" + token + ":s"},
		},
	}
	if len(params.ProposedExecpolicyAmendment) > 0 {
		buttons = append(buttons, []models.InlineKeyboardButton{
			{Text: "Allow similar", CallbackData: "ap:" + token + ":m"},
		})
	}
	buttons = append(buttons, []models.InlineKeyboardButton{
		{Text: "Decline", CallbackData: "ap:" + token + ":d"},
		{Text: "Cancel", CallbackData: "ap:" + token + ":c"},
	})

	if _, err := a.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: messageThreadID,
		Text:            text,
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: buttons,
		},
	}); err != nil {
		return nil, err
	}

	select {
	case decision := <-pending.ch:
		if decision.err != nil {
			return nil, decision.err
		}
		if decision.result == "amend" {
			return map[string]any{
				"decision": map[string]any{
					"acceptWithExecpolicyAmendment": map[string]any{
						"execpolicy_amendment": params.ProposedExecpolicyAmendment,
					},
				},
			}, nil
		}
		return decision.result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Minute):
		return nil, fmt.Errorf("approval timed out")
	}
}

func (a *app) requestPermissionApproval(ctx context.Context, chatID int64, messageThreadID int, req codex.ServerRequest) (any, error) {
	var params permissionRequestParams
	_ = json.Unmarshal(req.Params, &params)
	token := strconv.FormatInt(a.approvalSeq.Add(1), 36)
	pending := &pendingApproval{ch: make(chan approvalDecision, 1)}
	a.mu.Lock()
	a.approvals[token] = pending
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.approvals, token)
		a.mu.Unlock()
	}()

	text := permissionText(params)
	if _, err := a.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: messageThreadID,
		Text:            text,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "Allow turn", CallbackData: "ap:" + token + ":pt"},
				{Text: "Allow session", CallbackData: "ap:" + token + ":ps"},
			},
			{
				{Text: "Decline", CallbackData: "ap:" + token + ":d"},
				{Text: "Cancel", CallbackData: "ap:" + token + ":c"},
			},
		}},
	}); err != nil {
		return nil, err
	}

	select {
	case decision := <-pending.ch:
		if decision.err != nil {
			return nil, decision.err
		}
		switch decision.result {
		case "permission-turn":
			return map[string]any{"permissions": rawJSON(params.Permissions), "scope": "turn"}, nil
		case "permission-session":
			return map[string]any{"permissions": rawJSON(params.Permissions), "scope": "session"}, nil
		default:
			return decision.result, nil
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Minute):
		return nil, fmt.Errorf("permission approval timed out")
	}
}

func (a *app) requestUserInput(ctx context.Context, chatID int64, messageThreadID int, req codex.ServerRequest) (any, error) {
	var params userInputRequestParams
	_ = json.Unmarshal(req.Params, &params)
	if len(params.Questions) == 0 {
		return map[string]any{"answers": map[string]any{}}, nil
	}
	token := strconv.FormatInt(a.inputSeq.Add(1), 36)
	pending := &pendingInput{
		ch:              make(chan inputDecision, 1),
		chatID:          chatID,
		messageThreadID: messageThreadID,
		questions:       params.Questions,
	}

	msg, err := a.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: messageThreadID,
		Text:            userInputText(params.Questions),
		ReplyMarkup:     inputReplyMarkup(token, params.Questions),
	})
	if err != nil {
		return nil, err
	}
	pending.promptMessageID = msg.ID
	a.mu.Lock()
	a.inputs[token] = pending
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.inputs, token)
		a.mu.Unlock()
	}()

	select {
	case decision := <-pending.ch:
		if decision.err != nil {
			return nil, decision.err
		}
		return map[string]any{"answers": decision.result}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Minute):
		return nil, fmt.Errorf("user input timed out")
	}
}

func approvalText(method string, params approvalRequestParams) string {
	title := "Codex wants approval."
	if method == "item/commandExecution/requestApproval" {
		title = "Codex wants to run a command."
	}
	if method == "item/fileChange/requestApproval" {
		title = "Codex wants to edit files."
	}
	var parts []string
	parts = append(parts, title)
	if strings.TrimSpace(params.Reason) != "" {
		parts = append(parts, "Reason: "+strings.TrimSpace(params.Reason))
	}
	if strings.TrimSpace(params.Command) != "" {
		parts = append(parts, "Command:\n"+truncateMiddle(params.Command, 900))
	}
	return strings.Join(parts, "\n\n")
}

func permissionText(params permissionRequestParams) string {
	var parts []string
	parts = append(parts, "Codex wants additional permissions.")
	if params.Reason != nil && strings.TrimSpace(*params.Reason) != "" {
		parts = append(parts, "Reason: "+strings.TrimSpace(*params.Reason))
	}
	if strings.TrimSpace(params.CWD) != "" {
		parts = append(parts, "cwd: "+params.CWD)
	}
	if len(params.Permissions) > 0 {
		parts = append(parts, "Permissions:\n"+truncateMiddle(string(params.Permissions), 900))
	}
	return strings.Join(parts, "\n\n")
}

func userInputText(questions []inputQuestion) string {
	var parts []string
	parts = append(parts, "Codex needs input.")
	for _, q := range questions {
		label := strings.TrimSpace(q.Header)
		if label == "" {
			label = q.ID
		}
		parts = append(parts, label+": "+strings.TrimSpace(q.Question))
		if len(q.Options) > 0 {
			var opts []string
			for _, opt := range q.Options {
				opts = append(opts, "- "+opt.Label)
			}
			parts = append(parts, strings.Join(opts, "\n"))
		}
	}
	parts = append(parts, "Reply to this message with your answer. For multiple questions, use one line per answer.")
	return strings.Join(parts, "\n\n")
}

func inputReplyMarkup(token string, questions []inputQuestion) *models.InlineKeyboardMarkup {
	if len(questions) != 1 || len(questions[0].Options) == 0 {
		return nil
	}
	rows := make([][]models.InlineKeyboardButton, 0, len(questions[0].Options)+1)
	for i, opt := range questions[0].Options {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         opt.Label,
			CallbackData: "ui:" + token + ":" + strconv.Itoa(i),
		}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "Cancel", CallbackData: "ui:" + token + ":cancel"}})
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func rawJSON(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]any{}
	}
	var out any
	if json.Unmarshal(raw, &out) == nil && out != nil {
		return out
	}
	return map[string]any{}
}

func truncateMiddle(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	head := max / 2
	tail := max - head - 5
	return string(r[:head]) + "\n...\n" + string(r[len(r)-tail:])
}
