package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreUpsertGetAndUpdateConversation(t *testing.T) {
	store := openTestStore(t)
	defer closeTestStore(t, store)

	conv := Conversation{
		ChatID:           123,
		MessageThreadID:  7,
		CodexThreadID:    "thread-a",
		ProjectName:      "Dexgram",
		CWD:              `C:\work\dexgram`,
		Projectless:      true,
		TopicTitle:       "Build tests",
		TopicNamed:       true,
		LastSyncedTurnID: "turn-1",
	}
	if err := store.Upsert(conv); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.Get(123, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected conversation to be found")
	}
	assertConversationFields(t, got, conv)
	if got.UpdatedAt == "" {
		t.Fatal("expected UpdatedAt to be populated")
	}
	if _, err := time.Parse(time.RFC3339, got.UpdatedAt); err != nil {
		t.Fatalf("UpdatedAt is not RFC3339: %v", err)
	}

	updated := conv
	updated.CodexThreadID = "thread-b"
	updated.Projectless = false
	updated.TopicNamed = false
	updated.LastSyncedTurnID = "turn-2"
	if err := store.Upsert(updated); err != nil {
		t.Fatal(err)
	}

	got, ok, err = store.Get(123, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected updated conversation to be found")
	}
	assertConversationFields(t, got, updated)
}

func TestStoreGetMissingConversation(t *testing.T) {
	store := openTestStore(t)
	defer closeTestStore(t, store)

	if _, ok, err := store.Get(999, 1); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("expected missing conversation")
	}
}

func TestStoreStagedAttachmentsAreOrderedScopedAndClearable(t *testing.T) {
	store := openTestStore(t)
	defer closeTestStore(t, store)

	attachments := []StagedAttachment{
		{ChatID: 1, MessageThreadID: 10, MessageID: 100, Path: `C:\tmp\a.png`, Kind: "image", Name: "a.png"},
		{ChatID: 1, MessageThreadID: 10, MessageID: 101, Path: `C:\tmp\b.txt`, Kind: "file", Name: "b.txt"},
		{ChatID: 1, MessageThreadID: 11, MessageID: 102, Path: `C:\tmp\other.txt`, Kind: "file", Name: "other.txt"},
	}
	for _, attachment := range attachments {
		if err := store.AddStagedAttachment(attachment); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.ListStagedAttachments(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(got))
	}
	if got[0].ID == 0 || got[1].ID == 0 || got[0].ID >= got[1].ID {
		t.Fatalf("expected increasing database IDs, got %#v", got)
	}
	if got[0].Path != attachments[0].Path || got[1].Path != attachments[1].Path {
		t.Fatalf("attachments not returned in insert order: %#v", got)
	}
	if got[0].CreatedAt == "" || got[1].CreatedAt == "" {
		t.Fatalf("expected CreatedAt timestamps: %#v", got)
	}

	if err := store.ClearStagedAttachments(1, 10); err != nil {
		t.Fatal(err)
	}
	got, err = store.ListStagedAttachments(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected staged attachments to be cleared, got %#v", got)
	}

	other, err := store.ListStagedAttachments(1, 11)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 1 || other[0].Path != attachments[2].Path {
		t.Fatalf("expected other thread attachment to remain, got %#v", other)
	}
}

func TestStoreSettingsRoundTripAndMissing(t *testing.T) {
	store := openTestStore(t)
	defer closeTestStore(t, store)

	value, err := store.GetSetting("codex.model")
	if err != nil {
		t.Fatal(err)
	}
	if value != "" {
		t.Fatalf("missing setting = %q, want empty", value)
	}

	if err := store.SetSetting("codex.model", "gpt-test"); err != nil {
		t.Fatal(err)
	}
	value, err = store.GetSetting("codex.model")
	if err != nil {
		t.Fatal(err)
	}
	if value != "gpt-test" {
		t.Fatalf("setting = %q, want gpt-test", value)
	}

	if err := store.SetSetting("codex.model", ""); err != nil {
		t.Fatal(err)
	}
	value, err = store.GetSetting("codex.model")
	if err != nil {
		t.Fatal(err)
	}
	if value != "" {
		t.Fatalf("cleared setting = %q, want empty", value)
	}
}

func TestStoreTelegramPairingCodeConsumesOnce(t *testing.T) {
	store := openTestStore(t)
	defer closeTestStore(t, store)

	if err := store.SaveTelegramPairingCode("ABC234", -100123, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	chatID, ok, err := store.ConsumeTelegramPairingCode("ABC234")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || chatID != -100123 {
		t.Fatalf("pairing code = %d, %v; want -100123, true", chatID, ok)
	}
	if _, ok, err := store.ConsumeTelegramPairingCode("ABC234"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("expected consumed pairing code to be missing")
	}
}

func TestStoreTelegramPairingCodeExpires(t *testing.T) {
	store := openTestStore(t)
	defer closeTestStore(t, store)

	if err := store.SaveTelegramPairingCode("ABC234", 123, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.ConsumeTelegramPairingCode("ABC234"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("expected expired pairing code to be missing")
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "dexgram.db"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func closeTestStore(t *testing.T, store *Store) {
	t.Helper()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertConversationFields(t *testing.T, got, want Conversation) {
	t.Helper()
	if got.ChatID != want.ChatID ||
		got.MessageThreadID != want.MessageThreadID ||
		got.CodexThreadID != want.CodexThreadID ||
		got.ProjectName != want.ProjectName ||
		got.CWD != want.CWD ||
		got.Projectless != want.Projectless ||
		got.TopicTitle != want.TopicTitle ||
		got.TopicNamed != want.TopicNamed ||
		got.LastSyncedTurnID != want.LastSyncedTurnID {
		t.Fatalf("conversation mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}
