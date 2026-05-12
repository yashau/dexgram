package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"dexgram/internal/codex"
	"dexgram/internal/codexprojects"
	"dexgram/internal/codexstate"
	"dexgram/internal/state"

	"github.com/go-telegram/bot"
)

func (a *app) startTopicSession(ctx context.Context, key string, chatID int64, messageThreadID int, titleHint string) (*activeTurn, error) {
	sessionCtx, cancel := context.WithCancel(ctx)
	conv := a.topicConversation(chatID, messageThreadID)
	var err error
	conv, err = prepareProjectlessWorkspace(conv, titleHint)
	if err != nil {
		cancel()
		return nil, err
	}
	c, err := codex.StartStdioWithOptions(ctx, codex.StartOptions{
		CLIPath:    a.cfg.Codex.CLIPath,
		WorkingDir: appServerWorkingDir(conv),
	})
	if err != nil {
		cancel()
		return nil, err
	}
	go func() {
		for err := range c.Errors() {
			log.Printf("codex app-server: %v", err)
		}
	}()

	c.SetServerRequestHandler(func(_ context.Context, req codex.ServerRequest) (any, error) {
		return a.requestApproval(ctx, chatID, messageThreadID, req)
	})

	conv, err = a.prepareTopicConversation(ctx, c, conv)
	if err != nil {
		cancel()
		_ = c.Close()
		return nil, err
	}

	session := &activeTurn{
		client:   c,
		threadID: conv.CodexThreadID,
		ctx:      sessionCtx,
		cancel:   cancel,
		conv:     conv,
		turns:    map[string]*telegramTurn{},
	}
	if !a.registerSession(key, session) {
		cancel()
		_ = c.Close()
		return nil, fmt.Errorf("Codex is already running in this topic")
	}
	go a.collectTopicSession(sessionCtx, key, session)
	return session, nil
}

func (a *app) topicConversation(chatID int64, messageThreadID int) state.Conversation {
	conv, ok := a.store.Get(chatID, messageThreadID)
	if !ok {
		return state.Conversation{
			ChatID:          chatID,
			MessageThreadID: messageThreadID,
			Projectless:     true,
		}
	}
	if conv.ProjectName == "" {
		conv.Projectless = true
	}
	return conv
}

func (a *app) prepareTopicConversation(ctx context.Context, c *codex.Client, conv state.Conversation) (state.Conversation, error) {
	if conv.CodexThreadID == "" {
		threadID, cwd, err := a.startCodexThread(ctx, c, conv)
		if err != nil {
			return state.Conversation{}, err
		}
		conv.CodexThreadID = threadID
		conv.CWD = cwd
		if conv.Projectless {
			if err := codexstate.RegisterProjectlessThread(threadID, projectlessRoot()); err != nil {
				log.Printf("register projectless Codex thread: %v", err)
			}
		}
		if err := a.store.Upsert(conv); err != nil {
			return state.Conversation{}, err
		}
		log.Printf("mapped telegram thread %d:%d to codex thread %s", conv.ChatID, conv.MessageThreadID, threadID)
		return conv, nil
	}
	if err := a.resumeCodexThread(ctx, c, conv.CodexThreadID); err != nil {
		return state.Conversation{}, err
	}
	return conv, nil
}

func (a *app) startCodexThread(ctx context.Context, c *codex.Client, conv state.Conversation) (string, string, error) {
	params := map[string]any{
		"approvalPolicy": a.cfg.Codex.ApprovalPolicy,
		"sandbox":        a.cfg.Codex.Sandbox,
	}
	cwd := conv.CWD
	if cwd == "" {
		cwd = a.cfg.Codex.CWD
	}
	if cwd == "" {
		cwd = "."
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", err
	}
	params["cwd"] = abs
	var out codex.ThreadStartResponse
	if err := c.Call(ctx, "thread/start", params, &out); err != nil {
		return "", "", err
	}
	return out.Thread.ID, out.Cwd, nil
}

func (a *app) resumeCodexThread(ctx context.Context, c *codex.Client, threadID string) error {
	var out codex.ThreadResumeResponse
	return c.Call(ctx, "thread/resume", map[string]any{
		"threadId":       threadID,
		"approvalPolicy": a.cfg.Codex.ApprovalPolicy,
		"sandbox":        a.cfg.Codex.Sandbox,
	}, &out)
}

func (a *app) syncTopicTitle(ctx context.Context, conv *state.Conversation, c *codex.Client) error {
	if conv.TopicNamed {
		return nil
	}
	var read codex.ThreadReadResponse
	if err := c.Call(ctx, "thread/read", map[string]any{"threadId": conv.CodexThreadID}, &read); err != nil {
		return err
	}
	title := threadTitle(read.Thread)
	if title == "" {
		return nil
	}
	topicName := topicTitle(conv.ProjectName, title)
	if conv.TopicTitle == topicName {
		conv.TopicNamed = true
		return a.store.Upsert(*conv)
	}
	if _, err := a.bot.EditForumTopic(ctx, &bot.EditForumTopicParams{
		ChatID:          conv.ChatID,
		MessageThreadID: conv.MessageThreadID,
		Name:            topicName,
	}); err != nil {
		return err
	}
	conv.TopicTitle = topicName
	conv.TopicNamed = true
	return a.store.Upsert(*conv)
}

func projectlessRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Documents", "Codex")
}

func prepareProjectlessWorkspace(conv state.Conversation, titleHint string) (state.Conversation, error) {
	if !conv.Projectless || conv.CodexThreadID != "" || conv.CWD != "" {
		return conv, nil
	}
	root := projectlessRoot()
	if root == "" {
		return state.Conversation{}, fmt.Errorf("could not resolve Codex projectless workspace root")
	}
	dateDir := filepath.Join(root, time.Now().Format("2006-01-02"))
	slug := projectlessSlug(titleHint)
	if slug == "" {
		slug = "chat"
	}
	dir := filepath.Join(dateDir, slug)
	for i := 2; pathExists(dir); i++ {
		dir = filepath.Join(dateDir, fmt.Sprintf("%s-%d", slug, i))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return state.Conversation{}, fmt.Errorf("prepare projectless Codex workspace: %w", err)
	}
	conv.CWD = dir
	return conv, nil
}

func projectlessSlug(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	lastDash := false
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == '.':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 56 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func appServerWorkingDir(conv state.Conversation) string {
	if conv.Projectless {
		if conv.CWD != "" {
			return conv.CWD
		}
		return projectlessRoot()
	}
	if conv.CWD != "" {
		return conv.CWD
	}
	return projectlessRoot()
}

func (a *app) projectByIndex(index int) (codexprojects.Project, bool) {
	if index < 0 || index >= len(a.projects) {
		return codexprojects.Project{}, false
	}
	return a.projects[index], true
}

func (a *app) projectIndex(project codexprojects.Project) int {
	for i, candidate := range a.projects {
		if strings.EqualFold(candidate.Path, project.Path) {
			return i
		}
	}
	return -1
}

func startTurn(ctx context.Context, c *codex.Client, threadID string, input []map[string]any, approvalPolicy, sandbox string) (string, error) {
	var out codex.TurnStartResponse
	if err := c.Call(ctx, "turn/start", map[string]any{
		"threadId":       threadID,
		"input":          input,
		"approvalPolicy": approvalPolicy,
		"sandbox":        sandbox,
	}, &out); err != nil {
		return "", err
	}
	return out.Turn.ID, nil
}

func steerTurn(ctx context.Context, c *codex.Client, threadID, expectedTurnID string, input []map[string]any) error {
	if len(input) == 0 {
		input = []map[string]any{{
			"type":          "text",
			"text":          "",
			"text_elements": []any{},
		}}
	}
	var out struct {
		TurnID string `json:"turnId"`
	}
	return c.Call(ctx, "turn/steer", map[string]any{
		"threadId":       threadID,
		"expectedTurnId": expectedTurnID,
		"input":          input,
	}, &out)
}

func (a *app) setTopicGoal(ctx context.Context, chatID int64, messageThreadID int, objective string) error {
	key := fmt.Sprintf("%d:%d", chatID, messageThreadID)
	if session := a.activeSession(key); session != nil {
		return setThreadGoal(ctx, session.client, session.threadID, objective)
	}

	conv := a.topicConversation(chatID, messageThreadID)
	conv, err := prepareProjectlessWorkspace(conv, objective)
	if err != nil {
		return err
	}
	c, err := codex.StartStdioWithOptions(ctx, codex.StartOptions{
		CLIPath:    a.cfg.Codex.CLIPath,
		WorkingDir: appServerWorkingDir(conv),
	})
	if err != nil {
		return err
	}
	done := make(chan struct{})
	defer close(done)
	defer func() {
		_ = c.Close()
	}()
	errs := c.Errors()
	go func() {
		for {
			select {
			case err := <-errs:
				log.Printf("codex app-server: %v", err)
			case <-done:
				return
			}
		}
	}()
	c.SetServerRequestHandler(func(_ context.Context, req codex.ServerRequest) (any, error) {
		return a.requestApproval(ctx, chatID, messageThreadID, req)
	})
	conv, err = a.prepareTopicConversation(ctx, c, conv)
	if err != nil {
		return err
	}
	return setThreadGoal(ctx, c, conv.CodexThreadID, objective)
}

func setThreadGoal(ctx context.Context, c *codex.Client, threadID, objective string) error {
	var out map[string]any
	return c.Call(ctx, "thread/goal/set", map[string]any{
		"threadId":  threadID,
		"objective": objective,
	}, &out)
}

func textInput(prompt string) []map[string]any {
	input := []map[string]any{{
		"type":          "text",
		"text":          prompt,
		"text_elements": []any{},
	}}
	return input
}

func interruptTurn(ctx context.Context, c *codex.Client, threadID, turnID string) error {
	var out map[string]any
	return c.Call(ctx, "turn/interrupt", map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
	}, &out)
}

func (a *app) collectTopicSession(ctx context.Context, key string, session *activeTurn) {
	defer func() {
		a.release(key)
		session.cancel()
		_ = session.client.Close()
	}()

	for ev := range session.client.Events() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		switch ev.Method {
		case "turn/started":
			var started struct {
				ThreadID string     `json:"threadId"`
				Turn     codex.Turn `json:"turn"`
			}
			if json.Unmarshal(ev.Params, &started) == nil {
				if tgTurn := a.sessionTurn(key, started.Turn.ID); tgTurn != nil && tgTurn.StatusMessageID != 0 {
					_, _ = a.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
						ChatID:      tgTurn.ChatID,
						MessageID:   tgTurn.StatusMessageID,
						Text:        "Dexgram is thinking...",
						ReplyMarkup: turnControlMarkup(a.rememberTurnAction(key, tgTurn.TurnID), false),
					})
				}
			}
		case "item/started":
			var item codex.ItemStartedNotification
			if json.Unmarshal(ev.Params, &item) == nil {
				tgTurn := a.sessionTurn(key, item.TurnID)
				if tgTurn == nil {
					continue
				}
				if isCompactionNoticeItem(item.Item) {
					tgTurn.startCompactionDraft(ctx, a.bot, item.Item.ID)
				} else if isRunLogItem(item.Item) {
					tgTurn.ensureRunLog(ctx, a.bot)
					tgTurn.RunLog.start(item.Item)
				}
			}
		case "item/commandExecution/outputDelta", "item/fileChange/outputDelta":
			var delta codex.CommandOutputDeltaNotification
			if json.Unmarshal(ev.Params, &delta) == nil {
				if tgTurn := a.sessionTurn(key, delta.TurnID); tgTurn != nil && tgTurn.RunLog != nil {
					text := delta.Delta
					if text == "" {
						text = delta.Output
					}
					tgTurn.RunLog.output(delta.ItemID, text)
				}
			}
		case "item/agentMessage/delta", "item/plan/delta":
			var delta codex.AgentMessageDeltaNotification
			if json.Unmarshal(ev.Params, &delta) == nil {
				if tgTurn := a.sessionTurn(key, delta.TurnID); tgTurn != nil {
					tgTurn.Buffers[delta.ItemID] += delta.Delta
					if tgTurn.isCompactionItemID(delta.ItemID) {
						tgTurn.startCompactionDraft(ctx, a.bot, delta.ItemID)
						continue
					}
					if tgTurn.CompactionCancel != nil {
						tgTurn.stopCompactionDraft()
					}
					tgTurn.ensureInitial(ctx, a.bot)
					tgTurn.Initial.draft(tgTurn.Buffers[delta.ItemID])
				}
			}
		case "item/completed":
			var item codex.ItemCompletedNotification
			if json.Unmarshal(ev.Params, &item) != nil {
				continue
			}
			tgTurn := a.sessionTurn(key, item.TurnID)
			if tgTurn == nil {
				continue
			}
			if tgTurn.isCompactionItemID(item.Item.ID) || isCompactionNoticeItem(item.Item) {
				tgTurn.stopCompactionDraft()
				continue
			}
			switch item.Item.Type {
			case "agentMessage":
				tgTurn.LastAgent = item.Item.Text
				if item.Item.Phase != nil && *item.Item.Phase == "final_answer" {
					tgTurn.FinalAnswer = item.Item.Text
				} else if strings.TrimSpace(item.Item.Text) != "" {
					tgTurn.ensureInitial(ctx, a.bot)
					tgTurn.Initial.set(item.Item.Text)
				}
			case "plan":
				if strings.TrimSpace(item.Item.Text) != "" {
					tgTurn.ensureInitial(ctx, a.bot)
					tgTurn.Initial.set(item.Item.Text)
				}
			default:
				if isRunLogItem(item.Item) {
					tgTurn.ensureRunLog(ctx, a.bot)
					tgTurn.RunLog.complete(item.Item)
				}
				if item.Item.Type == "fileChange" || item.Item.Type == "imageGeneration" {
					a.sendCodexOutputs(ctx, tgTurn, session.conv, item.Item)
				}
			}
		case "turn/completed":
			var done codex.TurnCompletedNotification
			if json.Unmarshal(ev.Params, &done) != nil {
				continue
			}
			tgTurn := a.sessionTurn(key, done.Turn.ID)
			if tgTurn == nil {
				continue
			}
			answer := strings.TrimSpace(tgTurn.FinalAnswer)
			if answer == "" {
				answer = strings.TrimSpace(tgTurn.LastAgent)
			}
			if answer == "" {
				answer = "Codex completed without a final text answer."
			}
			tgTurn.stopCompactionDraft()
			if tgTurn.RunLog != nil {
				tgTurn.RunLog.finish()
			}
			if tgTurn.Initial != nil && sameTelegramText(tgTurn.Initial.text, answer) {
				tgTurn.Initial.delete()
			}
			sentAsNew := false
			if err := sendRichMessage(ctx, a.bot, tgTurn.ChatID, tgTurn.MessageThreadID, answer); err != nil {
				log.Printf("send final message: %v", err)
				if tgTurn.StatusMessageID != 0 {
					if editErr := editRichMessage(ctx, a.bot, tgTurn.ChatID, tgTurn.StatusMessageID, answer); editErr != nil {
						log.Printf("edit fallback final message: %v", editErr)
					}
				}
			} else {
				sentAsNew = true
			}
			if sentAsNew && tgTurn.StatusMessageID != 0 {
				if _, err := a.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
					ChatID:    tgTurn.ChatID,
					MessageID: tgTurn.StatusMessageID,
				}); err != nil {
					log.Printf("delete status message: %v", err)
				}
			}
			if err := a.syncTopicTitle(ctx, &session.conv, session.client); err != nil {
				log.Printf("rename telegram topic: %v", err)
			}
			session.conv.LastSyncedTurnID = done.Turn.ID
			_ = a.store.Upsert(session.conv)
			a.forgetTurnAction(key, tgTurn.TurnID)
			a.removeSessionTurn(key, tgTurn.TurnID)
			if a.sessionTurnCount(key) == 0 {
				return
			}
		case "error":
			log.Printf("codex app-server event error: %s", string(ev.Params))
		}
	}
}
