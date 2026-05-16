package main

import (
	"context"
	"reflect"
	"testing"

	"dexgram/internal/codex"
)

func TestUnsyncedTurnsReturnsLatestCompletedWhenNoMarker(t *testing.T) {
	turns := []codex.Turn{
		{ID: "t1", Status: "completed"},
		{ID: "t2", Status: "running"},
		{ID: "t3", Status: "completed"},
	}

	got := unsyncedTurns(turns, "")
	want := []codex.Turn{turns[2]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unsyncedTurns = %#v, want %#v", got, want)
	}
}

func TestUnsyncedTurnsAfterMarkerOnlyIncludesCompletedTurns(t *testing.T) {
	turns := []codex.Turn{
		{ID: "t1", Status: "completed"},
		{ID: "t2", Status: "running"},
		{ID: "t3", Status: "completed"},
		{ID: "t4", Status: "failed"},
		{ID: "t5", Status: "completed"},
	}

	got := unsyncedTurns(turns, "t2")
	want := []codex.Turn{turns[2], turns[4]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unsyncedTurns = %#v, want %#v", got, want)
	}
}

func TestSummarizeTurnSplitsInitialRunLogAndFinalAnswer(t *testing.T) {
	phase := "final_answer"
	turn := codex.Turn{Items: []codex.ThreadItem{
		{Type: "plan", Text: "Plan"},
		{Type: "commandExecution", Command: `powershell -Command "go test ./..."`},
		{Type: "agentMessage", Text: "Intermediate"},
		{Type: "agentMessage", Phase: &phase, Text: "Done"},
	}}

	initial, runLines, final := summarizeTurn(turn)
	if initial != "Plan\n\nIntermediate" {
		t.Fatalf("initial = %q", initial)
	}
	if !reflect.DeepEqual(runLines, []string{"shell: go test ./..."}) {
		t.Fatalf("runLines = %#v", runLines)
	}
	if final != "Done" {
		t.Fatalf("final = %q", final)
	}
}

func TestSummarizeTurnFallsBackToLastAgentMessage(t *testing.T) {
	turn := codex.Turn{Items: []codex.ThreadItem{
		{Type: "agentMessage", Text: "First"},
		{Type: "agentMessage", Text: "Last"},
	}}

	_, _, final := summarizeTurn(turn)
	if final != "Last" {
		t.Fatalf("final fallback = %q", final)
	}
}

func TestTurnUserPromptExtractsTextContent(t *testing.T) {
	turn := codex.Turn{Items: []codex.ThreadItem{
		{Type: "userMessage", Content: []byte(`[{"type":"text","text":"Desktop prompt\nsecond line","text_elements":[]}]`)},
		{Type: "agentMessage", Text: "answer"},
	}}

	if got := turnUserPrompt(turn); got != "Desktop prompt\nsecond line" {
		t.Fatalf("turnUserPrompt = %q", got)
	}
}

func TestTurnHasTelegramTranscriptPrompt(t *testing.T) {
	turn := codex.Turn{Items: []codex.ThreadItem{
		{Type: "userMessage", Content: []byte(`[{"type":"text","text":"Telegram: hello from chat\n","text_elements":[]}]`)},
		{Type: "agentMessage", Text: "answer"},
	}}
	if !turnHasTelegramTranscriptPrompt(turn) {
		t.Fatal("expected Telegram transcript prompt to be detected")
	}
}

func TestTurnHasTelegramTranscriptPromptIgnoresNonTelegramPrefix(t *testing.T) {
	turn := codex.Turn{Items: []codex.ThreadItem{
		{Type: "userMessage", Content: []byte(`[{"type":"text","text":"Telegraph: not the marker\n","text_elements":[]}]`)},
	}}
	if turnHasTelegramTranscriptPrompt(turn) {
		t.Fatal("unexpected Telegram transcript prompt detection")
	}
}

func TestTurnDesktopPromptSuppressesTelegramTranscript(t *testing.T) {
	turn := codex.Turn{Items: []codex.ThreadItem{
		{Type: "userMessage", Content: []byte(`[{"type":"text","text":"Telegram: hello from chat\n","text_elements":[]}]`)},
		{Type: "agentMessage", Text: "answer"},
	}}
	if got := turnDesktopPrompt(turn); got != "" {
		t.Fatalf("turnDesktopPrompt = %q, want empty", got)
	}
}

func TestPrefixQuotedPrompt(t *testing.T) {
	got := prefixQuotedPrompt("First line\n\nSecond line", "Answer")
	want := "> First line\n>\n> Second line\n\nAnswer"
	if got != want {
		t.Fatalf("prefixQuotedPrompt = %q, want %q", got, want)
	}
}

func TestStripAssistantAppDirectives(t *testing.T) {
	got := stripAssistantAppDirectives("Done.\n\n::git-stage{cwd=\"C:\\\\work\"}\n  ::archive{reason=\"done\"}\n\nStill visible.")
	want := "Done.\n\n\nStill visible."
	if got != want {
		t.Fatalf("stripAssistantAppDirectives = %q, want %q", got, want)
	}
}

func TestRenderHistoricalTurnQuotesUserPrompt(t *testing.T) {
	b, api := newTelegramTestBot(t)
	phase := "final_answer"
	turn := codex.Turn{ID: "t1", Status: "completed", Items: []codex.ThreadItem{
		{Type: "userMessage", Content: []byte(`[{"type":"text","text":"Desktop prompt","text_elements":[]}]`)},
		{Type: "agentMessage", Phase: &phase, Text: "Answer"},
	}}

	if err := renderHistoricalTurn(context.Background(), b, 123, 7, turn); err != nil {
		t.Fatal(err)
	}

	if !api.bodyContains("sendMessage", "Desktop prompt") {
		t.Fatalf("historical turn did not include prompt: %#v", api.calls)
	}
	if !api.bodyContains("sendMessage", "blockquote") {
		t.Fatalf("historical turn did not send blockquote entity: %#v", api.calls)
	}
}

func TestRenderHistoricalTurnStripsAppDirectives(t *testing.T) {
	b, api := newTelegramTestBot(t)
	phase := "final_answer"
	turn := codex.Turn{ID: "t1", Status: "completed", Items: []codex.ThreadItem{
		{Type: "userMessage", Content: []byte(`[{"type":"text","text":"Desktop prompt","text_elements":[]}]`)},
		{Type: "agentMessage", Phase: &phase, Text: "Done.\n\n::git-stage{cwd=\"C:\\\\work\"}\n::git-push{cwd=\"C:\\\\work\" branch=\"main\"}"},
	}}

	if err := renderHistoricalTurn(context.Background(), b, 123, 7, turn); err != nil {
		t.Fatal(err)
	}

	if !api.bodyContains("sendMessage", "Done.") {
		t.Fatalf("historical turn did not include final answer: %#v", api.calls)
	}
	if api.bodyContains("sendMessage", "::git-stage") || api.bodyContains("sendMessage", "::git-push") {
		t.Fatalf("historical turn included app directive: %#v", api.calls)
	}
}

func TestRenderHistoricalTurnSkipsRunLog(t *testing.T) {
	b, api := newTelegramTestBot(t)
	exitCode := 0
	phase := "final_answer"
	turn := codex.Turn{ID: "t1", Status: "completed", Items: []codex.ThreadItem{
		{Type: "commandExecution", Command: "go test ./...", ExitCode: &exitCode},
		{Type: "agentMessage", Phase: &phase, Text: "Done"},
	}}

	if err := renderHistoricalTurn(context.Background(), b, 123, 7, turn); err != nil {
		t.Fatal(err)
	}

	if api.bodyContains("sendMessage", "Synced run log") || api.bodyContains("sendMessage", "go test ./...") {
		t.Fatalf("historical turn synced run log: %#v", api.calls)
	}
	if !api.bodyContains("sendMessage", "Done") {
		t.Fatalf("historical turn did not send final answer: %#v", api.calls)
	}
}

func TestRenderHistoricalTurnSendsOnlyFinalReply(t *testing.T) {
	b, api := newTelegramTestBot(t)
	phase := "final_answer"
	turn := codex.Turn{ID: "t1", Status: "completed", Items: []codex.ThreadItem{
		{Type: "plan", Text: "Plan text"},
		{Type: "agentMessage", Text: "Intermediate draft"},
		{Type: "agentMessage", Phase: &phase, Text: "Final answer"},
	}}

	if err := renderHistoricalTurn(context.Background(), b, 123, 7, turn); err != nil {
		t.Fatal(err)
	}

	if api.bodyContains("sendMessage", "Plan text") || api.bodyContains("sendMessage", "Intermediate draft") {
		t.Fatalf("historical turn sent non-final content: %#v", api.calls)
	}
	if !api.bodyContains("sendMessage", "Final answer") {
		t.Fatalf("historical turn did not send final answer: %#v", api.calls)
	}
}

func TestRenderHistoricalTurnSilentDisablesNotification(t *testing.T) {
	b, api := newTelegramTestBot(t)
	phase := "final_answer"
	turn := codex.Turn{ID: "t1", Status: "completed", Items: []codex.ThreadItem{
		{Type: "agentMessage", Phase: &phase, Text: "Final answer"},
	}}

	if err := renderHistoricalTurnSilent(context.Background(), b, 123, 7, turn); err != nil {
		t.Fatal(err)
	}

	if !api.bodyContains("sendMessage", "name=\"disable_notification\"") || !api.bodyContains("sendMessage", "\r\ntrue\r\n") {
		t.Fatalf("silent historical turn did not disable notifications: %#v", api.calls)
	}
}

func TestShouldSkipTelegramOriginTurnUsesMarker(t *testing.T) {
	app := newHandlerTestApp(t, []int64{123})
	if err := app.store.SaveTelegramTurn("thread-a", "turn-1", 123, 7, 42); err != nil {
		t.Fatal(err)
	}
	if !app.shouldSkipTelegramOriginTurn("thread-a", codex.Turn{ID: "turn-1"}) {
		t.Fatal("expected marked telegram turn to be skipped")
	}
	if app.shouldSkipTelegramOriginTurn("thread-a", codex.Turn{ID: "turn-2"}) {
		t.Fatal("unexpected skip for unmarked turn")
	}
}

func TestParseSyncLimitDefaultsAndCaps(t *testing.T) {
	if got, err := parseSyncLimit(""); err != nil || got != defaultSyncTurnLimit {
		t.Fatalf("default sync limit = %d, %v", got, err)
	}
	if got, err := parseSyncLimit("100"); err != nil || got != maxSyncTurnLimit {
		t.Fatalf("capped sync limit = %d, %v", got, err)
	}
	if _, err := parseSyncLimit("nope"); err == nil {
		t.Fatal("expected invalid sync limit error")
	}
}

func TestRecentCompletedTurnsByMessageBudgetKeepsNewestWithinBudget(t *testing.T) {
	phase := "final_answer"
	exitCode := 0
	turns := []codex.Turn{
		{ID: "old", Status: "completed", Items: []codex.ThreadItem{{Type: "agentMessage", Phase: &phase, Text: "old"}}},
		{ID: "running", Status: "running", Items: []codex.ThreadItem{{Type: "agentMessage", Text: "skip"}}},
		{ID: "newer", Status: "completed", Items: []codex.ThreadItem{{Type: "plan", Text: "plan"}, {Type: "commandExecution", Command: "go test ./...", ExitCode: &exitCode}, {Type: "agentMessage", Phase: &phase, Text: "done"}}},
		{ID: "newest", Status: "completed", Items: []codex.ThreadItem{{Type: "agentMessage", Phase: &phase, Text: "latest"}}},
	}

	got := recentCompletedTurnsByMessageBudget(turns, 3)
	want := []codex.Turn{turns[0], turns[2], turns[3]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("recentCompletedTurnsByMessageBudget = %#v, want %#v", got, want)
	}
}
