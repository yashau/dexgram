package main

import (
	"path/filepath"
	"testing"

	"dexgram/internal/codex"

	"github.com/fsnotify/fsnotify"
)

func TestSessionFileEventMatches(t *testing.T) {
	path := filepath.Join("C:", "Users", "Yashau", ".codex", "sessions", "rollout-thread.jsonl")
	if !sessionFileEventMatches(fsnotify.Event{Name: path, Op: fsnotify.Write}, path) {
		t.Fatal("expected write event for session file to match")
	}
	if sessionFileEventMatches(fsnotify.Event{Name: path, Op: fsnotify.Chmod}, path) {
		t.Fatal("chmod should not trigger session mirror")
	}
	if sessionFileEventMatches(fsnotify.Event{Name: filepath.Join("C:", "other.jsonl"), Op: fsnotify.Write}, path) {
		t.Fatal("different file should not match")
	}
}

func TestCompletedTurnsAfterMarkerReportsMissingMarker(t *testing.T) {
	turns := []codex.Turn{{ID: "t1", Status: "completed"}, {ID: "t2", Status: "completed"}}
	got, found := completedTurnsAfterMarker(turns, "missing")
	if found {
		t.Fatal("expected missing marker to be reported")
	}
	if len(got) != 0 {
		t.Fatalf("expected no turns when marker is missing, got %#v", got)
	}
}
