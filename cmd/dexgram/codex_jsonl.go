package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/atij/jsonl"
)

var errNoCodexTranscriptBoundary = errors.New("no completed Codex turn boundary")

const telegramTranscriptPrefix = "Telegram:"

type codexJSONLRecord struct {
	Timestamp string             `json:"timestamp"`
	Type      string             `json:"type"`
	Dexgram   *codexJSONLDexgram `json:"dexgram,omitempty"`
	Payload   json.RawMessage    `json:"payload"`
}

type codexJSONLPayloadHeader struct {
	Type string `json:"type"`
	Role string `json:"role"`
}

type codexJSONLDexgram struct {
	Source          string `json:"source"`
	Kind            string `json:"kind"`
	ChatID          int64  `json:"chat_id"`
	MessageThreadID int    `json:"message_thread_id"`
	MessageID       int    `json:"message_id"`
	Version         int    `json:"version"`
}

type codexMessagePayload struct {
	Type    string                `json:"type"`
	Role    string                `json:"role"`
	Content []codexMessageContent `json:"content"`
}

type codexMessageContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexUserMessageEventPayload struct {
	Type         string `json:"type"`
	Message      string `json:"message"`
	Images       []any  `json:"images"`
	LocalImages  []any  `json:"local_images"`
	TextElements []any  `json:"text_elements"`
}

type bufferWriteCloser struct {
	*bytes.Buffer
}

func (b bufferWriteCloser) Close() error {
	return nil
}

func (a *app) syncTelegramPromptTranscript(chatID int64, messageThreadID, messageID int, threadID, text string) {
	if messageID == 0 || strings.TrimSpace(threadID) == "" || strings.TrimSpace(text) == "" {
		return
	}
	synced, err := a.store.HasTelegramTranscriptSync(chatID, messageThreadID, messageID)
	if err != nil {
		log.Printf("read telegram transcript sync marker chat_id=%d thread_id=%d message_id=%d: %v", chatID, messageThreadID, messageID, err)
		return
	}
	if synced {
		return
	}
	path, ok := codexSessionFilePath(threadID)
	if !ok {
		log.Printf("telegram transcript sync session file not found thread_id=%s", threadID)
		return
	}
	a.transcriptMu.Lock()
	defer a.transcriptMu.Unlock()
	meta := codexJSONLDexgram{
		Source:          "telegram",
		Kind:            "transcript_sync",
		ChatID:          chatID,
		MessageThreadID: messageThreadID,
		MessageID:       messageID,
		Version:         1,
	}
	if err := insertTelegramTranscriptRecord(path, text, meta, time.Now()); err != nil {
		if errors.Is(err, errNoCodexTranscriptBoundary) {
			log.Printf("telegram transcript sync skipped thread_id=%s path=%s: %v", threadID, path, err)
			return
		}
		log.Printf("telegram transcript sync failed thread_id=%s path=%s: %v", threadID, path, err)
		return
	}
	if err := a.store.SaveTelegramTranscriptSync(chatID, messageThreadID, messageID, threadID); err != nil {
		log.Printf("save telegram transcript sync marker chat_id=%d thread_id=%d message_id=%d: %v", chatID, messageThreadID, messageID, err)
	}
}

func telegramTranscriptText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, telegramTranscriptPrefix) {
		return text + "\n"
	}
	return telegramTranscriptPrefix + " " + text + "\n"
}

func insertTelegramTranscriptRecord(path, text string, meta codexJSONLDexgram, now time.Time) error {
	text = telegramTranscriptText(text)
	if text == "" {
		return nil
	}
	records, err := readCodexJSONL(path)
	if err != nil {
		return err
	}
	insertAt, timestamp, err := telegramTranscriptInsertPoint(records, now)
	if err != nil {
		return err
	}
	inserted, err := telegramTranscriptRecords(timestamp, text, meta)
	if err != nil {
		return err
	}
	next := make([]codexJSONLRecord, 0, len(records)+len(inserted))
	next = append(next, records[:insertAt]...)
	next = append(next, inserted...)
	next = append(next, records[insertAt:]...)
	return writeCodexJSONL(path, next)
}

func readCodexJSONL(path string) ([]codexJSONLRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()
	var records []codexJSONLRecord
	reader := jsonl.NewReader(f)
	if err := reader.ReadLines(func(data []byte) error {
		var record codexJSONLRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		records = append(records, record)
		return nil
	}); err != nil {
		return nil, err
	}
	return records, nil
}

func writeCodexJSONL(path string, records []codexJSONLRecord) error {
	var buf bytes.Buffer
	writer := jsonl.NewWriter(bufferWriteCloser{Buffer: &buf})
	for _, record := range records {
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

func telegramTranscriptInsertPoint(records []codexJSONLRecord, now time.Time) (int, string, error) {
	lastComplete := -1
	lastStarted := -1
	for i, record := range records {
		switch recordPayloadType(record) {
		case "task_complete":
			lastComplete = i
		case "task_started":
			lastStarted = i
		}
	}
	if lastComplete < 0 || lastStarted > lastComplete {
		return 0, "", errNoCodexTranscriptBoundary
	}
	insertAt := len(records)
	lastCompleteTime, err := time.Parse(time.RFC3339Nano, records[lastComplete].Timestamp)
	if err != nil {
		return 0, "", fmt.Errorf("parse task_complete timestamp: %w", err)
	}
	if !now.After(lastCompleteTime) {
		now = lastCompleteTime.Add(time.Millisecond)
	}
	return insertAt, formatCodexJSONLTimestamp(now), nil
}

func telegramTranscriptRecords(timestamp, text string, meta codexJSONLDexgram) ([]codexJSONLRecord, error) {
	messagePayload, err := json.Marshal(codexMessagePayload{
		Type: "message",
		Role: "user",
		Content: []codexMessageContent{{
			Type: "input_text",
			Text: text,
		}},
	})
	if err != nil {
		return nil, err
	}
	eventPayload, err := json.Marshal(codexUserMessageEventPayload{
		Type:         "user_message",
		Message:      text,
		Images:       []any{},
		LocalImages:  []any{},
		TextElements: []any{},
	})
	if err != nil {
		return nil, err
	}
	return []codexJSONLRecord{
		{Timestamp: timestamp, Type: "response_item", Dexgram: &meta, Payload: messagePayload},
		{Timestamp: timestamp, Type: "event_msg", Dexgram: &meta, Payload: eventPayload},
	}, nil
}

func recordPayloadType(record codexJSONLRecord) string {
	var header codexJSONLPayloadHeader
	if len(record.Payload) == 0 || json.Unmarshal(record.Payload, &header) != nil {
		return ""
	}
	return header.Type
}

func formatCodexJSONLTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
