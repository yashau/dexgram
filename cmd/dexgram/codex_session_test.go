package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dexgram/internal/state"
)

func TestProjectlessSlugNormalizesAndLimitsTitle(t *testing.T) {
	if got := projectlessSlug("  Build Dexgram.Tests_now!  "); got != "build-dexgram-tests-now" {
		t.Fatalf("projectlessSlug returned %q", got)
	}
	if got := projectlessSlug(strings.Repeat("a", 100)); len(got) != 56 {
		t.Fatalf("expected slug length 56, got %d: %q", len(got), got)
	}
	if got := projectlessSlug("!!!"); got != "" {
		t.Fatalf("expected empty slug, got %q", got)
	}
}

func TestPrepareProjectlessWorkspaceCreatesUniqueDirectory(t *testing.T) {
	home := setCmdTestHome(t)
	conv := state.Conversation{ChatID: 1, MessageThreadID: 2, Projectless: true}

	first, err := prepareProjectlessWorkspace(conv, "Chat Title")
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareProjectlessWorkspace(conv, "Chat Title")
	if err != nil {
		t.Fatal(err)
	}

	if first.CWD == "" || second.CWD == "" || first.CWD == second.CWD {
		t.Fatalf("expected unique workspaces, got %q and %q", first.CWD, second.CWD)
	}
	if _, err := os.Stat(first.CWD); err != nil {
		t.Fatalf("first workspace missing: %v", err)
	}
	if _, err := os.Stat(second.CWD); err != nil {
		t.Fatalf("second workspace missing: %v", err)
	}
	wantRoot := filepath.Join(home, "Documents", "Codex")
	if !strings.HasPrefix(first.CWD, wantRoot) {
		t.Fatalf("workspace %q is not under %q", first.CWD, wantRoot)
	}
}

func TestPrepareProjectlessWorkspaceSkipsAlreadyPreparedConversations(t *testing.T) {
	conv := state.Conversation{Projectless: false, CWD: `C:\work\dexgram`}
	got, err := prepareProjectlessWorkspace(conv, "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, conv) {
		t.Fatalf("conversation changed: %#v", got)
	}
}

func TestAppServerWorkingDirAndTextInput(t *testing.T) {
	projectless := state.Conversation{Projectless: true, CWD: `C:\workspace`}
	if got := appServerWorkingDir(projectless); got != `C:\workspace` {
		t.Fatalf("appServerWorkingDir projectless = %q", got)
	}
	project := state.Conversation{Projectless: false, CWD: `C:\project`}
	if got := appServerWorkingDir(project); got != `C:\project` {
		t.Fatalf("appServerWorkingDir project = %q", got)
	}

	want := []map[string]any{{
		"type":          "text",
		"text":          "hello",
		"text_elements": []any{},
	}}
	if got := textInput("hello"); !reflect.DeepEqual(got, want) {
		t.Fatalf("textInput = %#v", got)
	}
}

func setCmdTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}
