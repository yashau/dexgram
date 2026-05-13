package main

import (
	"strings"
	"testing"

	"dexgram/internal/codex"
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
