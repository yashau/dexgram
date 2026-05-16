package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"dexgram/internal/codex"

	"github.com/go-telegram/bot/models"
)

func TestRenderTelegramMessagesConvertsMarkdownToEntities(t *testing.T) {
	input := strings.Join([]string{
		"# Heading!",
		"- item_with [link](https://example.com/path) and `code_value`",
		"> quoted *text*",
		"plain (value).",
	}, "\n")

	messages := renderTelegramMessages(input, 4096)
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	got := messages[0]

	assertContains(t, got.Text, "Heading!")
	assertContains(t, got.Text, "item_with")
	assertContains(t, got.Text, "link")
	assertContains(t, got.Text, "code_value")
	assertContains(t, got.Text, "plain (value).")
	assertEntity(t, got, "bold")
	assertEntity(t, got, "text_link")
	assertEntity(t, got, "code")
}

func TestRenderTelegramMessagesCodeFenceUsesPreEntity(t *testing.T) {
	got := firstRenderedTelegramMessage("```go\nfmt.Println(`hello`)\n```", 4096)

	assertContains(t, got.Text, "fmt.Println(`hello`)")
	assertEntity(t, got, "pre")
}

func TestRenderTelegramMessagesDropsInvalidLocalTextLinks(t *testing.T) {
	got := firstRenderedTelegramMessage("> Desktop prompt\n\nChanged [telegram_handlers.go](C:/Users/Yashau/Projects/dexgram/cmd/dexgram/telegram_handlers.go:140).", 4096)

	assertContains(t, got.Text, "Desktop prompt")
	assertContains(t, got.Text, "telegram_handlers.go")
	assertEntity(t, got, "blockquote")
	assertNoEntity(t, got, "text_link")
}

func TestSplitTelegramChunksHandlesEmptyParagraphsAndLongText(t *testing.T) {
	if got := splitTelegramChunks("   ", 10); len(got) != 1 || got[0] != " " {
		t.Fatalf("empty text chunks = %#v", got)
	}

	got := splitTelegramChunks("alpha beta gamma delta", 12)
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks, got %#v", got)
	}
	for _, chunk := range got {
		if len([]rune(chunk)) > 12 {
			t.Fatalf("chunk too long: %q", chunk)
		}
	}
}

func TestRunLogLineFormatsKnownItems(t *testing.T) {
	exitCode := 0
	tests := []struct {
		item codex.ThreadItem
		want string
	}{
		{
			item: codex.ThreadItem{Type: "commandExecution", Command: `powershell -Command "go test ./..."`, ExitCode: &exitCode},
			want: "shell: go test ./...",
		},
		{
			item: codex.ThreadItem{Type: "commandExecution", Command: `Get-Content cmd\\dexgram\\main.go`, ExitCode: &exitCode},
			want: `shell: Get-Content cmd\dexgram\main.go`,
		},
		{
			item: codex.ThreadItem{Type: "fileChange", Changes: []codex.FileChange{{Path: `internal/state/store.go`}}},
			want: `edit: internal/state/store.go`,
		},
		{
			item: codex.ThreadItem{Type: "mcpToolCall", Server: "github", Tool: "fetch_pr"},
			want: "tool: github fetch_pr",
		},
		{
			item: codex.ThreadItem{Type: "webSearch", Query: "dexgram tests"},
			want: "web: dexgram tests",
		},
		{
			item: codex.ThreadItem{Type: "imageGeneration", SavedPath: `C:\tmp\image.png`},
			want: `image-gen: C:\tmp\image.png`,
		},
	}

	for _, test := range tests {
		if got := runLogLine(test.item); got != test.want {
			t.Fatalf("runLogLine(%#v) = %q, want %q", test.item, got, test.want)
		}
	}
}

func TestRunLogHelpersTrimAndTruncate(t *testing.T) {
	if got := trimCommandForLog(`powershell -c '& "go test ./..."'`, 80); got != "go test ./..." {
		t.Fatalf("trimCommandForLog returned %q", got)
	}
	if got := oneLine(" alpha\n\nbeta\tgamma ", 100); got != "alpha beta gamma" {
		t.Fatalf("oneLine collapsed to %q", got)
	}
	if got := oneLine("abcdefghijklmnopqrstuvwxyz", 10); got != "ab ... xyz" {
		t.Fatalf("oneLine truncated to %q", got)
	}
	if got := truncateRunLog("abcdefghijklmnopqrstuvwxyz", 10); got != "...\nuvwxyz" {
		t.Fatalf("truncateRunLog returned %q", got)
	}
}

func TestRenderMessageHashAndTextComparison(t *testing.T) {
	a := renderedTelegramMessage{Text: "hello", Entities: nil}
	b := renderedTelegramMessage{Text: "hello", Entities: nil}
	c := renderedTelegramMessage{Text: "hello", Entities: []models.MessageEntity{{Type: "bold", Offset: 0, Length: 5}}}

	if telegramMessageHash(a) != telegramMessageHash(b) {
		t.Fatal("identical rendered messages should hash equally")
	}
	if telegramMessageHash(a) == telegramMessageHash(c) {
		t.Fatal("message entities should affect hash")
	}
	if !sameTelegramText("line\r\nnext", "line\nnext") {
		t.Fatal("CRLF and LF text should compare equal")
	}
	if sameTelegramText("line one", "line two") {
		t.Fatal("different text should not compare equal")
	}
}

func TestLiveTextAndRunLogSendEditAndDelete(t *testing.T) {
	oldQueue := defaultTelegramQueue
	defaultTelegramQueue = newTelegramQueue(0)
	defer func() {
		defaultTelegramQueue = oldQueue
	}()

	b, api := newTelegramTestBot(t)
	turn := &telegramTurn{ChatID: 123, MessageThreadID: 7}
	turn.ensureInitial(context.Background(), b)
	turn.ensureInitial(context.Background(), b)
	turn.Initial.set("Initial message")
	turn.Initial.set("Edited message")
	turn.Initial.delete()
	if turn.Initial.messageID != 0 {
		t.Fatalf("initial message id after delete = %d", turn.Initial.messageID)
	}

	turn.ensureRunLog(context.Background(), b)
	turn.ensureRunLog(context.Background(), b)
	turn.RunLog.start(codex.ThreadItem{ID: "cmd-1", Type: "commandExecution", Command: "go test ./..."})
	turn.RunLog.output("cmd-1", "ignored output")
	turn.RunLog.lastFlush = time.Now().Add(-runLogMinInterval)
	exitCode := 0
	turn.RunLog.complete(codex.ThreadItem{ID: "cmd-1", Type: "commandExecution", Command: "go test ./cmd/dexgram", ExitCode: &exitCode})
	turn.RunLog.finish()

	if api.count("sendMessage") < 2 {
		t.Fatalf("sendMessage count = %d, want at least 2", api.count("sendMessage"))
	}
	if api.count("editMessageText") < 2 {
		t.Fatalf("editMessageText count = %d, want at least 2", api.count("editMessageText"))
	}
	if api.count("deleteMessage") < 2 {
		t.Fatalf("deleteMessage count = %d, want at least 2", api.count("deleteMessage"))
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func assertEntity(t *testing.T, message renderedTelegramMessage, entityType string) {
	t.Helper()
	for _, entity := range message.Entities {
		if string(entity.Type) == entityType {
			return
		}
	}
	t.Fatalf("expected entity %q in %#v", entityType, message.Entities)
}

func assertNoEntity(t *testing.T, message renderedTelegramMessage, entityType string) {
	t.Helper()
	for _, entity := range message.Entities {
		if string(entity.Type) == entityType {
			t.Fatalf("unexpected entity %q in %#v", entityType, message.Entities)
		}
	}
}
