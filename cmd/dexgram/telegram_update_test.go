package main

import (
	"path/filepath"
	"strings"
	"testing"

	"dexgram/internal/config"
	"dexgram/internal/state"
)

func TestPendingUpdateNoticeRoundTrip(t *testing.T) {
	store, err := state.Open(filepath.Join(t.TempDir(), "dexgram.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	app := &app{cfg: &config.Config{}, store: store}
	notice := pendingUpdateNotice{
		ChatID:          123,
		MessageThreadID: 45,
		FromVersion:     "0.2.2",
		TargetVersion:   "0.2.3",
		StartedAt:       "2026-05-14T12:00:00Z",
	}

	if err := app.savePendingUpdateNotice(notice); err != nil {
		t.Fatal(err)
	}
	got, ok, err := app.loadPendingUpdateNotice()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected pending update notice")
	}
	if got != notice {
		t.Fatalf("notice mismatch\ngot:  %#v\nwant: %#v", got, notice)
	}

	if err := app.clearPendingUpdateNotice(); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := app.loadPendingUpdateNotice(); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("expected pending update notice to be cleared")
	}
}

func TestPendingUpdateMessages(t *testing.T) {
	notice := pendingUpdateNotice{FromVersion: "0.2.2", TargetVersion: "0.2.3"}
	restart := pendingUpdateRestartMessage(notice)
	if !strings.Contains(restart, "Restarting Dexgram for update") ||
		!strings.Contains(restart, "0.2.2 -> 0.2.3") {
		t.Fatalf("unexpected restart message: %q", restart)
	}

	complete := pendingUpdateCompleteMessage(notice, "0.2.3")
	if !strings.Contains(complete, "Dexgram is back") || !strings.Contains(complete, "0.2.3") {
		t.Fatalf("unexpected complete message: %q", complete)
	}
}

func TestCleanVersion(t *testing.T) {
	if got := cleanVersion(" v0.2.3 "); got != "0.2.3" {
		t.Fatalf("cleanVersion() = %q, want 0.2.3", got)
	}
}
