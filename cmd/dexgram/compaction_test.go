package main

import (
	"testing"

	"dexgram/internal/codex"
)

func TestIsCompactionNoticeItem(t *testing.T) {
	if !isCompactionNoticeItem(codex.ThreadItem{Type: "compaction"}) {
		t.Fatalf("compaction item was not recognized")
	}
	if isCompactionNoticeItem(codex.ThreadItem{Type: "agentMessage", Text: "Compacting context..."}) {
		t.Fatalf("agent message text should not be treated as structured compaction")
	}
}

func TestCompactionDraftFrame(t *testing.T) {
	want := []string{
		"Compacting context.",
		"Compacting context..",
		"Compacting context...",
		"Compacting context.",
	}
	for i, expected := range want {
		if got := compactionDraftFrame(i); got != expected {
			t.Fatalf("compactionDraftFrame(%d) = %q, want %q", i, got, expected)
		}
	}
}
