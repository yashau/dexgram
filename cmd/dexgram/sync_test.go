package main

import (
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
