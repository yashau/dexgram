package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dexgram/internal/codexprojects"
	"dexgram/internal/config"
	"dexgram/internal/state"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type telegramAPICall struct {
	Method string
	Body   string
}

type telegramAPIServer struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []telegramAPICall
}

func newTelegramTestBot(t *testing.T) (*bot.Bot, *telegramAPIServer) {
	t.Helper()
	api := &telegramAPIServer{}
	api.server = httptest.NewServer(http.HandlerFunc(api.handle))
	t.Cleanup(api.server.Close)

	b, err := bot.New("test-token", bot.WithServerURL(api.server.URL))
	if err != nil {
		t.Fatalf("create test bot: %v", err)
	}
	return b, api
}

func (s *telegramAPIServer) handle(w http.ResponseWriter, r *http.Request) {
	method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	s.mu.Lock()
	s.calls = append(s.calls, telegramAPICall{Method: method, Body: string(body)})
	s.mu.Unlock()

	var result any = true
	switch method {
	case "getMe":
		result = map[string]any{"id": 1, "is_bot": true, "first_name": "Dexgram"}
	case "sendMessage", "editMessageText", "editMessageReplyMarkup":
		result = map[string]any{"message_id": 100, "date": 1, "chat": map[string]any{"id": 123, "type": "private"}}
	case "createForumTopic":
		result = map[string]any{"message_thread_id": 77, "name": "New chat"}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func (s *telegramAPIServer) count(method string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, call := range s.calls {
		if call.Method == method {
			count++
		}
	}
	return count
}

func (s *telegramAPIServer) bodyContains(method, text string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, call := range s.calls {
		if call.Method == method && strings.Contains(call.Body, text) {
			return true
		}
	}
	return false
}

func newHandlerTestApp(t *testing.T, chatIDs []int64) *app {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "dexgram.db"))
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	t.Cleanup(func() {
		closeTestStateStore(t, store)
	})
	return &app{
		cfg: &config.Config{Telegram: config.TelegramConfig{
			ChatIDs: chatIDs,
		}},
		store:                 store,
		configPath:            filepath.Join(t.TempDir(), "dexgram.toml"),
		active:                map[string]*activeTurn{},
		actions:               map[string]turnAction{},
		approvals:             map[string]*pendingApproval{},
		inputs:                map[string]*pendingInput{},
		typingSuppressedUntil: map[string]time.Time{},
	}
}

func TestHandleUpdateUnauthorizedSendsPairingReply(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{999})

	app.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		ID:              5,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "hello",
	}})

	if api.count("sendMessage") != 1 {
		t.Fatalf("sendMessage count = %d, want 1", api.count("sendMessage"))
	}
	if !api.bodyContains("sendMessage", "Pairing code") {
		t.Fatalf("sendMessage body did not include pairing guidance: %#v", api.calls)
	}
}

func TestHandleUpdateRoutesPlanUsageAndStatus(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})

	app.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "/plan",
	}})
	if !api.bodyContains("sendMessage", "Usage: /plan <message>") {
		t.Fatalf("plan usage was not sent: %#v", api.calls)
	}

	if err := app.store.Upsert(state.Conversation{
		ChatID:          123,
		MessageThreadID: 7,
		CodexThreadID:   "thread-1",
		ProjectName:     "Dexgram",
		CWD:             `C:\dexgram`,
	}); err != nil {
		t.Fatalf("upsert conversation: %v", err)
	}
	app.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "/status",
	}})
	if !api.bodyContains("sendMessage", "thread-1") {
		t.Fatalf("status message did not include thread id: %#v", api.calls)
	}
}

func TestHandleUpdateUnknownCommandDoesNotBecomePrompt(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})

	app.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		ID:              5,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "/frobnicate asd",
	}})

	if api.count("sendMessage") != 1 {
		t.Fatalf("sendMessage count = %d, want 1", api.count("sendMessage"))
	}
	if !api.bodyContains("sendMessage", "Unknown Dexgram command: /frobnicate") {
		t.Fatalf("unknown command reply was not sent: %#v", api.calls)
	}
	if api.bodyContains("sendMessage", "How should Dexgram use this message?") {
		t.Fatalf("unknown command fell through to prompt handling: %#v", api.calls)
	}
}

func TestHandleUpdateTopicCommandIsIgnored(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})

	app.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		ID:              5,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "/topic",
	}})

	if api.count("sendMessage") != 0 {
		t.Fatalf("sendMessage count = %d, want 0; calls: %#v", api.count("sendMessage"), api.calls)
	}
}

func TestHandleUpdateTopicCommandWithTextAsksFreshTopicChoice(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})

	app.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		ID:              5,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "/topic asd",
	}})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !api.bodyContains("sendMessage", "How should Dexgram use this message?") {
		time.Sleep(10 * time.Millisecond)
	}
	if !api.bodyContains("sendMessage", "How should Dexgram use this message?") {
		t.Fatalf("fresh topic choice was not sent: %#v", api.calls)
	}
	if !api.bodyContains("sendMessage", "Resume session") || !api.bodyContains("sendMessage", "Start new chat") {
		t.Fatalf("fresh topic buttons missing: %#v", api.calls)
	}
}

func TestFreshUnboundTopicAsksHowToUseFirstMessage(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})

	app.handlePrompt(context.Background(), b, &models.Message{
		ID:              10,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "please start here",
	}, "please start here")

	if !api.bodyContains("sendMessage", "How should Dexgram use this message?") {
		t.Fatalf("fresh topic choice was not sent: %#v", api.calls)
	}
	if !api.bodyContains("sendMessage", "Resume session") || !api.bodyContains("sendMessage", "Start new chat") {
		t.Fatalf("fresh topic buttons missing: %#v", api.calls)
	}
}

func TestFreshTopicCommandNewCallbackIsSilent(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	token := app.rememberFreshTopic(&pendingFreshTopic{
		chatID:          123,
		messageThreadID: 7,
		replyMessageID:  5,
		input:           []map[string]any{{"type": "text", "text": "/topic asd"}},
		displayText:     "/topic asd",
		createdAt:       time.Now(),
	})

	app.handleFreshTopicCallback(context.Background(), b, callbackQuery("fresh:"+token+":new"))

	if api.count("sendMessage") != 0 {
		t.Fatalf("sendMessage count = %d, want 0; calls: %#v", api.count("sendMessage"), api.calls)
	}
	if api.bodyContains("editMessageText", "Not submitting") {
		t.Fatalf("fresh command callback sent obsolete message: %#v", api.calls)
	}
	if api.count("editMessageReplyMarkup") != 1 {
		t.Fatalf("editMessageReplyMarkup count = %d, want 1; calls: %#v", api.count("editMessageReplyMarkup"), api.calls)
	}
}

func TestHandleSideCommandRequiresStartedCodexThread(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	if err := app.store.Upsert(state.Conversation{
		ChatID:          123,
		MessageThreadID: 7,
		ProjectName:     "Dexgram",
		CWD:             `C:\work\dexgram`,
	}); err != nil {
		t.Fatal(err)
	}

	app.handleSideCommand(context.Background(), b, &models.Message{
		ID:              5,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "/side check this",
	}, "check this")

	if api.count("createForumTopic") != 0 {
		t.Fatalf("createForumTopic count = %d, want 0", api.count("createForumTopic"))
	}
	if !api.bodyContains("sendMessage", "Start this Codex chat first") {
		t.Fatalf("sendMessage body did not include guard text: %#v", api.calls)
	}
}

func TestHandleUpdateStoresForumTopicRename(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	if err := app.store.Upsert(state.Conversation{
		ChatID:          123,
		MessageThreadID: 7,
		CodexThreadID:   "thread-1",
		TopicTitle:      "Old title",
	}); err != nil {
		t.Fatal(err)
	}

	app.handleUpdate(context.Background(), b, &models.Update{Message: &models.Message{
		ID:               6,
		MessageThreadID:  7,
		Chat:             models.Chat{ID: 123},
		ForumTopicEdited: &models.ForumTopicEdited{Name: "Manual topic name"},
	}})

	conv, ok, err := app.store.Get(123, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || conv.TopicTitle != "Manual topic name" || !conv.TopicNamed {
		t.Fatalf("conversation rename not stored: ok=%v conv=%#v", ok, conv)
	}
	if api.count("sendMessage") != 0 {
		t.Fatalf("rename service message should not send Telegram messages: %#v", api.calls)
	}
}

func TestHandlePendingInputReplyBuildsAnswers(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	ch := make(chan inputDecision, 1)
	app.inputs["input-token"] = &pendingInput{
		ch:              ch,
		chatID:          123,
		messageThreadID: 7,
		promptMessageID: 50,
		questions: []inputQuestion{
			{ID: "name"},
			{ID: "color"},
		},
	}

	handled := app.handlePendingInputReply(context.Background(), b, &models.Message{
		ID:              55,
		MessageThreadID: 7,
		Chat:            models.Chat{ID: 123},
		Text:            "Alice\nBlue",
		ReplyToMessage:  &models.Message{ID: 50},
	})
	if !handled {
		t.Fatal("expected pending input reply to be handled")
	}
	decision := <-ch
	if got := decision.result["name"].(map[string]any)["answers"].([]string)[0]; got != "Alice" {
		t.Fatalf("name answer = %q", got)
	}
	if got := decision.result["color"].(map[string]any)["answers"].([]string)[0]; got != "Blue" {
		t.Fatalf("color answer = %q", got)
	}
	if !api.bodyContains("sendMessage", "Answered Codex input request.") {
		t.Fatalf("input confirmation was not sent: %#v", api.calls)
	}
}

func TestHandlerCallbacksResolveApprovalInputAndQueuedDelete(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})

	approvalCh := make(chan approvalDecision, 1)
	app.approvals["approval-token"] = &pendingApproval{ch: approvalCh}
	app.handleApprovalCallback(context.Background(), b, callbackQuery("ap:approval-token:s"))
	if decision := <-approvalCh; decision.result.(map[string]any)["decision"] != "acceptForSession" {
		t.Fatalf("approval decision = %#v", decision.result)
	}

	inputCh := make(chan inputDecision, 1)
	app.inputs["ui-token"] = &pendingInput{
		ch: inputCh,
		questions: []inputQuestion{{
			ID: "mode",
			Options: []inputOption{
				{Label: "Fast"},
				{Label: "Careful"},
			},
		}},
	}
	app.handleUserInputCallback(context.Background(), b, callbackQuery("ui:ui-token:1"))
	if got := (<-inputCh).result["mode"].(map[string]any)["answers"].([]string)[0]; got != "Careful" {
		t.Fatalf("input callback answer = %q", got)
	}

	key := "123:7"
	session := &activeTurn{turns: map[string]*telegramTurn{}}
	if !app.registerSession(key, session) {
		t.Fatal("register session")
	}
	app.addSessionTurn(key, &telegramTurn{TurnID: "active"})
	app.addSessionTurn(key, &telegramTurn{TurnID: "queued", Queued: true})
	app.actions["delete-token"] = turnAction{Key: key, TurnID: "queued"}
	app.handleDeleteQueuedCallback(context.Background(), b, callbackQuery("dq:delete-token"))
	if got := app.sessionTurn(key, "queued"); got != nil {
		t.Fatalf("queued turn still present: %#v", got)
	}
	if api.count("answerCallbackQuery") < 3 {
		t.Fatalf("answerCallbackQuery count = %d, want at least 3", api.count("answerCallbackQuery"))
	}
}

func TestProjectStatusAndNewTopicHandlersUseTelegramAPI(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	project := codexprojects.Project{Name: "Dexgram", Path: `C:\dexgram`}

	app.setTopicProject(context.Background(), b, 123, 7, project, true)
	conv, ok, err := app.store.Get(123, 7)
	if err != nil || !ok {
		t.Fatalf("stored project conv ok=%v err=%v", ok, err)
	}
	if conv.ProjectName != "Dexgram" || conv.CWD != `C:\dexgram` || conv.Projectless {
		t.Fatalf("stored project conv = %#v", conv)
	}

	app.handleStatusCommand(context.Background(), b, &models.Message{Chat: models.Chat{ID: 123}, MessageThreadID: 7})
	app.handleProjectCommand(context.Background(), b, &models.Message{Chat: models.Chat{ID: 123}, MessageThreadID: 7}, "/project")
	app.handleNewCommand(context.Background(), b, &models.Message{Chat: models.Chat{ID: 123}, MessageThreadID: 7}, "/new Follow up")

	newConv, ok, err := app.store.Get(123, 77)
	if err != nil || !ok {
		t.Fatalf("stored new topic conv ok=%v err=%v", ok, err)
	}
	if !newConv.Projectless || newConv.TopicTitle != "New chat" {
		t.Fatalf("new topic conv = %#v", newConv)
	}
	if api.count("createForumTopic") != 1 {
		t.Fatalf("createForumTopic count = %d, want 1", api.count("createForumTopic"))
	}
	if !api.bodyContains("sendMessage", "Usage: /project <project name>") {
		t.Fatalf("project usage message was not sent: %#v", api.calls)
	}
}

func TestProjectCommandRefreshesProjectsAndSelectsSingleMatch(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	writeCodexProjectState(t,
		`C:\Users\Yashau\Projects\dexgram`,
		`C:\Users\Yashau\Projects\other`,
	)

	app.handleProjectCommand(context.Background(), b, &models.Message{
		Chat:            models.Chat{ID: 123},
		MessageThreadID: 7,
	}, "/project dexgram")

	conv, ok, err := app.store.Get(123, 7)
	if err != nil || !ok {
		t.Fatalf("stored project conv ok=%v err=%v", ok, err)
	}
	if conv.ProjectName != "dexgram" || !strings.Contains(strings.ToLower(conv.CWD), "dexgram") {
		t.Fatalf("stored project conv = %#v", conv)
	}
	if !api.bodyContains("sendMessage", "Project set: dexgram") {
		t.Fatalf("project selection message was not sent: %#v", api.calls)
	}
}

func TestProjectCommandAmbiguousMatchShowsSelectionButtons(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	writeCodexProjectState(t,
		`C:\Users\Yashau\Projects\dexgram-api`,
		`C:\Users\Yashau\Projects\dexgram-ui`,
	)

	app.handleProjectCommand(context.Background(), b, &models.Message{
		Chat:            models.Chat{ID: 123},
		MessageThreadID: 7,
	}, "/project dexgram")

	if !api.bodyContains("sendMessage", "Select the Codex project for this chat:") {
		t.Fatalf("ambiguous project message was not sent: %#v", api.calls)
	}
}

func TestNewCommandWithAmbiguousProjectSendsGuidance(t *testing.T) {
	b, api := newTelegramTestBot(t)
	app := newHandlerTestApp(t, []int64{123})
	writeCodexProjectState(t,
		`C:\Users\Yashau\Projects\dexgram-api`,
		`C:\Users\Yashau\Projects\dexgram-ui`,
	)

	app.handleNewCommand(context.Background(), b, &models.Message{
		Chat:            models.Chat{ID: 123},
		MessageThreadID: 7,
	}, "/new dexgram: investigate")

	if api.count("createForumTopic") != 0 {
		t.Fatalf("createForumTopic count = %d, want 0", api.count("createForumTopic"))
	}
	if !api.bodyContains("sendMessage", "That project name is ambiguous.") {
		t.Fatalf("ambiguous new-project guidance was not sent: %#v", api.calls)
	}
}

func writeCodexProjectState(t *testing.T, paths ...string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	stateDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create codex state dir: %v", err)
	}
	raw, err := json.Marshal(map[string]any{"project-order": paths})
	if err != nil {
		t.Fatalf("marshal project state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, ".codex-global-state.json"), raw, 0o600); err != nil {
		t.Fatalf("write project state: %v", err)
	}
}

func callbackQuery(data string) *models.CallbackQuery {
	return &models.CallbackQuery{
		ID:   "callback-id",
		Data: data,
		Message: models.MaybeInaccessibleMessage{
			Type: models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{
				ID:              99,
				MessageThreadID: 7,
				Date:            1,
				Chat:            models.Chat{ID: 123},
			},
		},
	}
}
