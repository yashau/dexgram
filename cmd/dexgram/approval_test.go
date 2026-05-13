package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestApprovalTextIncludesRelevantDetails(t *testing.T) {
	got := approvalText("item/commandExecution/requestApproval", approvalRequestParams{
		Reason:  " needs tests ",
		Command: "go test ./...",
	})

	assertContains(t, got, "Codex wants to run a command.")
	assertContains(t, got, "Reason: needs tests")
	assertContains(t, got, "Command:\ngo test ./...")
}

func TestApprovalTextPrefersCommandActions(t *testing.T) {
	got := approvalText("item/commandExecution/requestApproval", approvalRequestParams{
		Command: "powershell -Command \"git status\"",
		CommandActions: []struct {
			Command string `json:"command"`
		}{
			{Command: "git status"},
			{Command: "go test ./..."},
		},
		GrantRoot: `C:\work\dexgram`,
	})

	assertContains(t, got, "Command:\ngit status && go test ./...")
	if strings.Contains(got, "powershell -Command") {
		t.Fatalf("approval text used raw shell wrapper: %s", got)
	}
	assertContains(t, got, `Requested writable root: C:\work\dexgram`)
}

func TestPermissionTextIncludesReasonCWDAndPermissions(t *testing.T) {
	reason := "needs workspace write"
	got := permissionText(permissionRequestParams{
		CWD:         `C:\work\dexgram`,
		Reason:      &reason,
		Permissions: json.RawMessage(`{"write":true}`),
	})

	assertContains(t, got, "Codex wants additional permissions.")
	assertContains(t, got, "Reason: needs workspace write")
	assertContains(t, got, `cwd: C:\work\dexgram`)
	assertContains(t, got, `Permissions:
{"write":true}`)
}

func TestUserInputTextAndReplyMarkup(t *testing.T) {
	questions := []inputQuestion{{
		ID:       "choice",
		Header:   "Mode",
		Question: "Pick one",
		Options: []inputOption{
			{Label: "Fast"},
			{Label: "Careful"},
		},
	}}

	text := userInputText(questions)
	assertContains(t, text, "Codex needs input.")
	assertContains(t, text, "Mode: Pick one")
	assertContains(t, text, "- Fast")
	assertContains(t, text, "Reply to this message with your answer.")

	markup := inputReplyMarkup("tok", questions)
	if markup == nil || len(markup.InlineKeyboard) != 3 {
		t.Fatalf("unexpected input markup: %#v", markup)
	}
	if markup.InlineKeyboard[0][0].CallbackData != "ui:tok:0" {
		t.Fatalf("unexpected first callback: %#v", markup.InlineKeyboard[0][0])
	}
	if markup.InlineKeyboard[2][0].CallbackData != "ui:tok:cancel" {
		t.Fatalf("unexpected cancel callback: %#v", markup.InlineKeyboard[2][0])
	}

	if inputReplyMarkup("tok", append(questions, inputQuestion{ID: "other"})) != nil {
		t.Fatal("expected no inline markup for multiple questions")
	}
}

func TestRawJSONAndTruncateMiddle(t *testing.T) {
	got := rawJSON(json.RawMessage(`{"a":1}`))
	if !reflect.DeepEqual(got, map[string]any{"a": float64(1)}) {
		t.Fatalf("rawJSON decoded %#v", got)
	}
	if got := rawJSON(json.RawMessage(`{`)); !reflect.DeepEqual(got, map[string]any{}) {
		t.Fatalf("invalid rawJSON fallback = %#v", got)
	}

	short := truncateMiddle("  short  ", 20)
	if short != "short" {
		t.Fatalf("truncateMiddle short = %q", short)
	}
	long := truncateMiddle(strings.Repeat("a", 20), 10)
	if !strings.Contains(long, "\n...\n") || len([]rune(long)) != 10 {
		t.Fatalf("truncateMiddle long = %q", long)
	}
}
