package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dexgram/internal/codex"
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

func TestTelegramPromptInputPrefixesFirstTextItem(t *testing.T) {
	input := []map[string]any{
		{
			"type":          "text",
			"text":          "hello",
			"text_elements": []any{},
		},
		{
			"type": "localImage",
			"path": `C:\photo.jpg`,
		},
	}

	got := telegramPromptInput(input)
	if got[0]["text"] != "Telegram: hello" {
		t.Fatalf("telegramPromptInput text = %q", got[0]["text"])
	}
	if input[0]["text"] != "hello" {
		t.Fatalf("telegramPromptInput mutated original input: %#v", input)
	}
	if got[1]["path"] != `C:\photo.jpg` {
		t.Fatalf("telegramPromptInput attachment = %#v", got[1])
	}
}

func TestTelegramPromptInputDoesNotDoublePrefix(t *testing.T) {
	got := telegramPromptInput(textInput("Telegram: hello"))
	if got[0]["text"] != "Telegram: hello" {
		t.Fatalf("telegramPromptInput text = %q", got[0]["text"])
	}
}

func TestTelegramStartedFinalAnswerDoesNotQuotePrompt(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	app.bot = b
	key := "123:7"
	session := &activeTurn{
		conv: state.Conversation{
			ChatID:          123,
			MessageThreadID: 7,
			CodexThreadID:   "thread-1",
			TopicNamed:      true,
		},
		turns: map[string]*telegramTurn{},
	}
	if !app.registerSession(key, session) {
		t.Fatal("register session")
	}
	app.addSessionTurn(key, &telegramTurn{
		TurnID:          "turn-1",
		ChatID:          123,
		MessageThreadID: 7,
		SourceMessageID: 55,
		Text:            "Telegram prompt",
		FinalAnswer:     "Final answer",
		Buffers:         map[string]string{},
		SentFiles:       map[string]bool{},
	})

	params, err := json.Marshal(codex.TurnCompletedNotification{
		ThreadID: "thread-1",
		Turn:     codex.Turn{ID: "turn-1", Status: "completed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	app.handleTopicSessionEvent(context.Background(), key, session, codex.Event{
		Method: "turn/completed",
		Params: params,
	})

	if !api.bodyContains("sendMessage", "Final answer") {
		t.Fatalf("final answer was not sent: %#v", api.calls)
	}
	if api.bodyContains("sendMessage", "Telegram prompt") || api.bodyContains("sendMessage", "blockquote") {
		t.Fatalf("telegram-origin final answer quoted prompt: %#v", api.calls)
	}
	if !api.bodyContains("sendMessage", "reply_parameters") || !api.bodyContains("sendMessage", "55") {
		t.Fatalf("telegram-origin final answer did not reply to source message: %#v", api.calls)
	}
}

func TestAutonomousTurnStartedRendersFinalAnswer(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	app.bot = b
	key := "123:7"
	session := &activeTurn{
		threadID: "thread-1",
		conv: state.Conversation{
			ChatID:          123,
			MessageThreadID: 7,
			CodexThreadID:   "thread-1",
			TopicNamed:      true,
		},
		turns: map[string]*telegramTurn{},
	}
	if !app.registerSession(key, session) {
		t.Fatal("register session")
	}

	app.handleTopicSessionEvent(context.Background(), key, session, mustJSON(t, struct {
		ThreadID string     `json:"threadId"`
		Turn     codex.Turn `json:"turn"`
	}{ThreadID: "thread-1", Turn: codex.Turn{ID: "goal-turn"}}).event("turn/started"))

	phase := "final_answer"
	app.handleTopicSessionEvent(context.Background(), key, session, mustJSON(t, codex.ItemCompletedNotification{
		ThreadID: "thread-1",
		TurnID:   "goal-turn",
		Item:     codex.ThreadItem{Type: "agentMessage", Phase: &phase, Text: "Autonomous final"},
	}).event("item/completed"))
	app.handleTopicSessionEvent(context.Background(), key, session, mustJSON(t, codex.TurnCompletedNotification{
		ThreadID: "thread-1",
		Turn:     codex.Turn{ID: "goal-turn", Status: "completed"},
	}).event("turn/completed"))

	if !api.bodyContains("sendMessage", "Autonomous final") {
		t.Fatalf("autonomous final answer was not sent: %#v", api.calls)
	}
	if api.bodyContains("sendMessage", "reply_parameters") {
		t.Fatalf("autonomous final answer unexpectedly replied to a user message: %#v", api.calls)
	}
}

func TestAutonomousEmptyTurnDoesNotSendCompletionMessage(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	app.bot = b
	key := "123:7"
	session := &activeTurn{
		threadID: "thread-1",
		conv: state.Conversation{
			ChatID:          123,
			MessageThreadID: 7,
			CodexThreadID:   "thread-1",
			TopicNamed:      true,
		},
		turns: map[string]*telegramTurn{},
	}
	if !app.registerSession(key, session) {
		t.Fatal("register session")
	}

	app.handleTopicSessionEvent(context.Background(), key, session, mustJSON(t, struct {
		ThreadID string     `json:"threadId"`
		Turn     codex.Turn `json:"turn"`
	}{ThreadID: "thread-1", Turn: codex.Turn{ID: "empty-goal-turn"}}).event("turn/started"))
	app.handleTopicSessionEvent(context.Background(), key, session, mustJSON(t, codex.TurnCompletedNotification{
		ThreadID: "thread-1",
		Turn:     codex.Turn{ID: "empty-goal-turn", Status: "completed"},
	}).event("turn/completed"))

	if api.count("sendMessage") != 0 {
		t.Fatalf("empty autonomous turn sent Telegram messages: %#v", api.calls)
	}
}

type testEventPayload []byte

func mustJSON(t *testing.T, value any) testEventPayload {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (p testEventPayload) event(method string) codex.Event {
	return codex.Event{Method: method, Params: json.RawMessage(p)}
}

func TestTopicConversationDefaultsAndStoredMappings(t *testing.T) {
	store, err := state.Open(filepath.Join(t.TempDir(), "dexgram.db"))
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer closeTestStateStore(t, store)
	app := &app{store: store}

	conv := app.topicConversation(123, 7)
	if conv.ChatID != 123 || conv.MessageThreadID != 7 || !conv.Projectless {
		t.Fatalf("new topic conversation = %#v", conv)
	}

	if err := store.Upsert(state.Conversation{
		ChatID:          123,
		MessageThreadID: 7,
		CodexThreadID:   "thread-1",
		CWD:             `C:\work`,
	}); err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}
	conv = app.topicConversation(123, 7)
	if !conv.Projectless {
		t.Fatalf("stored topic without project should be projectless: %#v", conv)
	}

	if err := store.Upsert(state.Conversation{
		ChatID:          123,
		MessageThreadID: 8,
		ProjectName:     "Dexgram",
		CWD:             `C:\dexgram`,
	}); err != nil {
		t.Fatalf("upsert project conversation: %v", err)
	}
	conv = app.topicConversation(123, 8)
	if conv.Projectless || conv.ProjectName != "Dexgram" {
		t.Fatalf("stored project topic = %#v", conv)
	}
}

func setCmdTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}
