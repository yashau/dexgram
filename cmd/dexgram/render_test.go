package main

import (
	"strings"
	"testing"

	"dexgram/internal/codex"
)

func TestRenderTelegramMarkdownEscapesTextAndPreservesSupportedMarkdown(t *testing.T) {
	input := strings.Join([]string{
		"# Heading!",
		"- item_with [link](https://example.com/path) and `code_value`",
		"> quoted *text*",
		"plain (value).",
	}, "\n")

	got := renderTelegramMarkdown(input)

	assertContains(t, got, "*Heading\\!*")
	assertContains(t, got, "\u2022 item\\_with [link](https://example.com/path) and `code_value`")
	assertContains(t, got, "> quoted \\*text\\*")
	assertContains(t, got, "plain \\(value\\)\\.")
}

func TestRenderTelegramMarkdownCodeFenceEscapesCodeOnly(t *testing.T) {
	got := renderTelegramMarkdown("```go\nfmt.Println(`hello`)\n```")

	if got != "```go\nfmt.Println(\\`hello\\`)\n```" {
		t.Fatalf("unexpected rendered fence: %q", got)
	}
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
