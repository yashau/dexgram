package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInsertTelegramTranscriptRecordAppendsAfterCompletedTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	initial := []codexJSONLRecord{
		recordWithPayload(t, "2026-05-15T10:00:00.000Z", "event_msg", `{"type":"task_started"}`),
		recordWithPayload(t, "2026-05-15T10:00:01.000Z", "response_item", `{"type":"message","role":"user","content":[{"type":"input_text","text":"hello\n"}]}`),
		recordWithPayload(t, "2026-05-15T10:00:02.000Z", "event_msg", `{"type":"task_complete"}`),
	}
	if err := writeCodexJSONL(path, initial); err != nil {
		t.Fatal(err)
	}

	meta := codexJSONLDexgram{Source: "telegram", Kind: "transcript_sync", ChatID: 123, MessageThreadID: 7, MessageID: 42, Version: 1}
	if err := insertTelegramTranscriptRecord(path, "hello from tg\n", meta, mustTime(t, "2026-05-15T10:00:03.000Z")); err != nil {
		t.Fatal(err)
	}

	records, err := readCodexJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 5 {
		t.Fatalf("records = %d, want 5", len(records))
	}
	assertRecord(t, records[3], "2026-05-15T10:00:03.000Z", "response_item", "message", "user")
	assertRecord(t, records[4], "2026-05-15T10:00:03.000Z", "event_msg", "user_message", "")
	if got := recordMessageText(t, records[3]); got != "Telegram: hello from tg\n" {
		t.Fatalf("message text = %q", got)
	}
	if got := recordEventMessage(t, records[4]); got != "Telegram: hello from tg\n" {
		t.Fatalf("event message = %q", got)
	}
	if records[3].Dexgram == nil || records[3].Dexgram.MessageID != 42 {
		t.Fatalf("missing dexgram metadata: %#v", records[3].Dexgram)
	}
	if records[4].Dexgram == nil || records[4].Dexgram.Source != "telegram" {
		t.Fatalf("missing event dexgram metadata: %#v", records[4].Dexgram)
	}
}

func TestInsertTelegramTranscriptRecordRefusesActiveTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	records := []codexJSONLRecord{
		recordWithPayload(t, "2026-05-15T10:00:00.000Z", "event_msg", `{"type":"task_complete"}`),
		recordWithPayload(t, "2026-05-15T10:00:01.000Z", "event_msg", `{"type":"task_started"}`),
	}
	if err := writeCodexJSONL(path, records); err != nil {
		t.Fatal(err)
	}

	err := insertTelegramTranscriptRecord(path, "Telegram: nope\n", codexJSONLDexgram{}, mustTime(t, "2026-05-15T10:00:02.000Z"))
	if !errors.Is(err, errNoCodexTranscriptBoundary) {
		t.Fatalf("error = %v, want errNoCodexTranscriptBoundary", err)
	}
}

func TestTelegramTranscriptTextPrefixesTelegram(t *testing.T) {
	got := telegramTranscriptText(" hello\nworld ")
	want := "Telegram: hello\nworld\n"
	if got != want {
		t.Fatalf("telegramTranscriptText = %q, want %q", got, want)
	}
}

func TestTelegramTranscriptTextDoesNotDoublePrefix(t *testing.T) {
	got := telegramTranscriptText(" Telegram: hello ")
	want := "Telegram: hello\n"
	if got != want {
		t.Fatalf("telegramTranscriptText = %q, want %q", got, want)
	}
}

func recordWithPayload(t *testing.T, timestamp, typ, payload string) codexJSONLRecord {
	t.Helper()
	if !json.Valid([]byte(payload)) {
		t.Fatalf("invalid payload: %s", payload)
	}
	return codexJSONLRecord{Timestamp: timestamp, Type: typ, Payload: json.RawMessage(payload)}
}

func assertRecord(t *testing.T, record codexJSONLRecord, timestamp, typ, payloadType, role string) {
	t.Helper()
	if record.Timestamp != timestamp || record.Type != typ {
		t.Fatalf("record = %#v, want timestamp=%s type=%s", record, timestamp, typ)
	}
	var header codexJSONLPayloadHeader
	if err := json.Unmarshal(record.Payload, &header); err != nil {
		t.Fatal(err)
	}
	if header.Type != payloadType || header.Role != role {
		t.Fatalf("payload header = %#v, want type=%s role=%s", header, payloadType, role)
	}
}

func recordMessageText(t *testing.T, record codexJSONLRecord) string {
	t.Helper()
	var payload codexMessagePayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Content) != 1 {
		t.Fatalf("content length = %d, want 1", len(payload.Content))
	}
	return payload.Content[0].Text
}

func recordEventMessage(t *testing.T, record codexJSONLRecord) string {
	t.Helper()
	var payload codexUserMessageEventPayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Message
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestReadCodexJSONLIgnoresEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	records, err := readCodexJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %d, want 0", len(records))
	}
}
