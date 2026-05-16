package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"dexgram/internal/codex"
	"dexgram/internal/state"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	sessionProjectsPerPage = 8
	sessionThreadsPerPage  = 8
	sessionListPageSize    = 100
	sessionListMaxThreads  = 1000
)

func (a *app) handleSessionsCommand(ctx context.Context, b *bot.Bot, msg *models.Message, query string) {
	browser, token, err := a.newSessionBrowser(ctx, msg.Chat.ID, msg.MessageThreadID, "", query)
	if err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Could not load Codex sessions:\n\n" + err.Error(),
		})
		return
	}
	text, markup := a.sessionBrowserProjectView(token, browser)
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:              msg.Chat.ID,
		MessageThreadID:     msg.MessageThreadID,
		Text:                text,
		ReplyMarkup:         markup,
		DisableNotification: true,
	})
}

func (a *app) newSessionBrowser(ctx context.Context, chatID int64, messageThreadID int, pendingFreshKey, query string) (*sessionBrowser, string, error) {
	threads, err := a.fetchCodexThreads(ctx, strings.TrimSpace(query))
	if err != nil {
		return nil, "", err
	}
	browser := &sessionBrowser{
		chatID:          chatID,
		messageThreadID: messageThreadID,
		pendingFreshKey: pendingFreshKey,
		query:           strings.TrimSpace(query),
		threads:         threads,
		projects:        groupSessionProjects(threads, a.projectsSnapshot()),
		projectIndex:    -1,
		createdAt:       time.Now(),
	}
	token := strconv.FormatInt(a.sessionBrowserSeq.Add(1), 36)
	a.mu.Lock()
	if a.sessionBrowsers == nil {
		a.sessionBrowsers = map[string]*sessionBrowser{}
	}
	a.sessionBrowsers[token] = browser
	a.mu.Unlock()
	return browser, token, nil
}

func (a *app) sessionBrowser(token string) (*sessionBrowser, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	browser := a.sessionBrowsers[token]
	return browser, browser != nil
}

func (a *app) forgetSessionBrowser(token string) {
	a.mu.Lock()
	delete(a.sessionBrowsers, token)
	a.mu.Unlock()
}

func (a *app) fetchCodexThreads(ctx context.Context, query string) ([]codex.Thread, error) {
	c, err := codex.StartStdioWithOptions(ctx, codex.StartOptions{
		CLIPath:    a.cfg.Codex.CLIPath,
		WorkingDir: a.cfg.Codex.CWD,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = c.Close()
	}()
	go func() {
		for err := range c.Errors() {
			log.Printf("codex app-server: %v", err)
		}
	}()

	threads, err := fetchThreadListPages(ctx, c, query, true)
	if err != nil {
		threads, err = fetchThreadListPages(ctx, c, query, false)
		if err != nil {
			return nil, err
		}
	}
	if query != "" {
		threads = filterThreads(threads, query)
	}
	sort.SliceStable(threads, func(i, j int) bool {
		return threads[i].UpdatedAt > threads[j].UpdatedAt
	})
	return threads, nil
}

func fetchThreadListPages(ctx context.Context, c *codex.Client, query string, includeSourceKinds bool) ([]codex.Thread, error) {
	var threads []codex.Thread
	var cursor any
	for len(threads) < sessionListMaxThreads {
		params := map[string]any{"limit": sessionListPageSize}
		if includeSourceKinds {
			params["sourceKinds"] = []string{"cli", "vscode", "appServer", "exec", "unknown"}
		}
		if query != "" {
			params["searchTerm"] = query
		}
		if cursor != nil {
			params["cursor"] = cursor
		}
		var out codex.ThreadListResponse
		if err := c.Call(ctx, "thread/list", params, &out); err != nil {
			return nil, err
		}
		threads = append(threads, threadListItems(out)...)
		next := threadListCursor(out)
		if next == nil {
			break
		}
		cursor = next
	}
	if len(threads) > sessionListMaxThreads {
		threads = threads[:sessionListMaxThreads]
	}
	return threads, nil
}

func threadListItems(out codex.ThreadListResponse) []codex.Thread {
	switch {
	case len(out.Data) > 0:
		return out.Data
	case len(out.Items) > 0:
		return out.Items
	default:
		return out.Threads
	}
}

func threadListCursor(out codex.ThreadListResponse) any {
	for _, raw := range []json.RawMessage{out.NextCursor, out.NextCursorAlt} {
		trimmed := strings.TrimSpace(string(raw))
		if trimmed == "" || trimmed == "null" {
			continue
		}
		var value any
		if err := json.Unmarshal(raw, &value); err == nil && value != nil {
			return value
		}
		return trimmed
	}
	return nil
}

func filterThreads(threads []codex.Thread, query string) []codex.Thread {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return threads
	}
	out := make([]codex.Thread, 0, len(threads))
	for _, thread := range threads {
		haystack := strings.ToLower(strings.Join([]string{
			thread.ID,
			threadTitle(thread),
			thread.Preview,
			thread.Cwd,
		}, "\n"))
		if strings.Contains(haystack, query) {
			out = append(out, thread)
		}
	}
	return out
}

func groupSessionProjects(threads []codex.Thread, projects []codexProjectLabel) []sessionProjectGroup {
	labelsByPath := map[string]string{}
	for _, project := range projects {
		labelsByPath[normalizeSessionPath(project.Path)] = project.Name
	}
	indexByPath := map[string]int{}
	var groups []sessionProjectGroup
	for i, thread := range threads {
		path := strings.TrimSpace(thread.Cwd)
		key := normalizeSessionPath(path)
		if key == "" {
			key = "<projectless>"
		}
		index, ok := indexByPath[key]
		if !ok {
			name := labelsByPath[key]
			if name == "" {
				name = sessionProjectName(path)
			}
			index = len(groups)
			indexByPath[key] = index
			groups = append(groups, sessionProjectGroup{Name: name, CWD: path})
		}
		groups[index].ThreadCount++
		groups[index].ThreadIDs = append(groups[index].ThreadIDs, i)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		left := newestThreadUpdatedAt(threads, groups[i])
		right := newestThreadUpdatedAt(threads, groups[j])
		if left == right {
			return groups[i].Name < groups[j].Name
		}
		return left > right
	})
	return groups
}

func newestThreadUpdatedAt(threads []codex.Thread, group sessionProjectGroup) float64 {
	var newest float64
	for _, id := range group.ThreadIDs {
		if id >= 0 && id < len(threads) && threads[id].UpdatedAt > newest {
			newest = threads[id].UpdatedAt
		}
	}
	return newest
}

type codexProjectLabel struct {
	Name string
	Path string
}

func (a *app) projectsSnapshot() []codexProjectLabel {
	a.projectsMu.RLock()
	defer a.projectsMu.RUnlock()
	out := make([]codexProjectLabel, 0, len(a.projects))
	for _, project := range a.projects {
		out = append(out, codexProjectLabel{Name: project.Name, Path: project.Path})
	}
	return out
}

func normalizeSessionPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	cleaned := filepath.Clean(path)
	return strings.ToLower(strings.TrimRight(cleaned, `\/`))
}

func sessionProjectName(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "Projectless"
	}
	name := filepath.Base(filepath.Clean(path))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return path
	}
	return name
}

func (a *app) sessionBrowserProjectView(token string, browser *sessionBrowser) (string, *models.InlineKeyboardMarkup) {
	total := len(browser.projects)
	if total == 0 {
		text := "No Codex sessions found."
		if browser.query != "" {
			text = fmt.Sprintf("No Codex sessions matched %q.", browser.query)
		}
		return text, &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: "Refresh", CallbackData: "sess:" + token + ":refresh"},
		}}}
	}
	browser.projectIndex = -1
	start, end := pageBounds(browser.page, sessionProjectsPerPage, total)
	lines := []string{"Codex sessions"}
	if browser.query != "" {
		lines = append(lines, "Filter: "+browser.query)
	}
	lines = append(lines, fmt.Sprintf("Projects %d-%d of %d", start+1, end, total), "")
	rows := [][]models.InlineKeyboardButton{}
	for i := start; i < end; i++ {
		project := browser.projects[i]
		lines = append(lines, fmt.Sprintf("%d. %s (%d)", i+1, project.Name, project.ThreadCount))
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         shortButtonLabel(fmt.Sprintf("%s (%d)", project.Name, project.ThreadCount)),
			CallbackData: fmt.Sprintf("sess:%s:proj:%d", token, i),
		}})
	}
	rows = append(rows, sessionNavRows(token, browser.page, sessionProjectsPerPage, total, "projects")...)
	return strings.Join(lines, "\n"), &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (a *app) sessionBrowserThreadView(token string, browser *sessionBrowser) (string, *models.InlineKeyboardMarkup) {
	if browser.projectIndex < 0 || browser.projectIndex >= len(browser.projects) {
		return a.sessionBrowserProjectView(token, browser)
	}
	project := browser.projects[browser.projectIndex]
	total := len(project.ThreadIDs)
	start, end := pageBounds(browser.page, sessionThreadsPerPage, total)
	lines := []string{project.Name, fmt.Sprintf("Threads %d-%d of %d", start+1, end, total), ""}
	rows := [][]models.InlineKeyboardButton{}
	for i := start; i < end; i++ {
		threadIndex := project.ThreadIDs[i]
		thread := browser.threads[threadIndex]
		title := threadTitle(thread)
		if title == "" {
			title = "Untitled session"
		}
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, title))
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         shortButtonLabel(title),
			CallbackData: fmt.Sprintf("sess:%s:attach:%d", token, threadIndex),
		}})
	}
	rows = append(rows, sessionNavRows(token, browser.page, sessionThreadsPerPage, total, "threads")...)
	rows = append(rows, []models.InlineKeyboardButton{{Text: "Back to projects", CallbackData: "sess:" + token + ":back"}})
	return strings.Join(lines, "\n"), &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func pageBounds(page, perPage, total int) (int, int) {
	if perPage <= 0 {
		perPage = 1
	}
	maxPage := max((total-1)/perPage, 0)
	page = min(max(page, 0), maxPage)
	start := page * perPage
	end := min(start+perPage, total)
	return start, end
}

func sessionNavRows(token string, page, perPage, total int, scope string) [][]models.InlineKeyboardButton {
	var rows [][]models.InlineKeyboardButton
	maxPage := max((total-1)/perPage, 0)
	var nav []models.InlineKeyboardButton
	if page > 0 {
		nav = append(nav, models.InlineKeyboardButton{Text: "Prev", CallbackData: fmt.Sprintf("sess:%s:page:%s:%d", token, scope, page-1)})
	}
	if page < maxPage {
		nav = append(nav, models.InlineKeyboardButton{Text: "Next", CallbackData: fmt.Sprintf("sess:%s:page:%s:%d", token, scope, page+1)})
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "Refresh", CallbackData: "sess:" + token + ":refresh"}})
	return rows
}

func (a *app) handleFreshTopicCallback(ctx context.Context, b *bot.Bot, query *models.CallbackQuery) {
	parts := strings.Split(query.Data, ":")
	if len(parts) != 3 {
		return
	}
	token, action := parts[1], parts[2]
	switch action {
	case "new":
		pending, ok := a.takeFreshTopic(token)
		if !ok {
			_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "That pending message expired.", ShowAlert: true})
			return
		}
		if name, ok := pendingFreshTopicCommandName(pending); ok {
			if query.Message.Message != nil {
				_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
					ChatID:    query.Message.Message.Chat.ID,
					MessageID: query.Message.Message.ID,
					Text:      fmt.Sprintf("Not submitting /%s to Codex because it is a Telegram command.", name),
				})
			}
			_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
			return
		}
		if query.Message.Message != nil {
			_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID:    query.Message.Message.Chat.ID,
				MessageID: query.Message.Message.ID,
				Text:      "Starting a new Codex chat.",
			})
		}
		a.submitBuiltPrompt(ctx, b, pending.chatID, pending.messageThreadID, pending.replyMessageID, pending.input, pending.displayText, pending.collaborationMode)
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Starting new chat."})
	case "sessions":
		pending, ok := a.freshTopic(token)
		if !ok {
			_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "That pending message expired.", ShowAlert: true})
			return
		}
		browser, browserToken, err := a.newSessionBrowser(ctx, pending.chatID, pending.messageThreadID, token, "")
		if err != nil {
			_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Could not load sessions.", ShowAlert: true})
			return
		}
		text, markup := a.sessionBrowserProjectView(browserToken, browser)
		if query.Message.Message != nil {
			_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID:      query.Message.Message.Chat.ID,
				MessageID:   query.Message.Message.ID,
				Text:        text,
				ReplyMarkup: markup,
			})
		}
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	case "project":
		_, _ = a.takeFreshTopic(token)
		if query.Message.Message != nil {
			_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID:    query.Message.Message.Chat.ID,
				MessageID: query.Message.Message.ID,
				Text:      "Run /project <project name>, then send your message again.",
			})
		}
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	}
}

func (a *app) handleSessionBrowserCallback(ctx context.Context, b *bot.Bot, query *models.CallbackQuery) {
	if query.Message.Message == nil {
		return
	}
	parts := strings.Split(query.Data, ":")
	if len(parts) < 3 {
		return
	}
	token := parts[1]
	browser, ok := a.sessionBrowser(token)
	if !ok {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "That session browser expired.", ShowAlert: true})
		return
	}
	action := parts[2]
	switch action {
	case "proj":
		if len(parts) != 4 {
			return
		}
		index, err := strconv.Atoi(parts[3])
		if err != nil || index < 0 || index >= len(browser.projects) {
			return
		}
		browser.projectIndex = index
		browser.page = 0
		text, markup := a.sessionBrowserThreadView(token, browser)
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: query.Message.Message.Chat.ID, MessageID: query.Message.Message.ID, Text: text, ReplyMarkup: markup})
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	case "page":
		if len(parts) != 5 {
			return
		}
		page, err := strconv.Atoi(parts[4])
		if err != nil {
			return
		}
		browser.page = page
		var text string
		var markup *models.InlineKeyboardMarkup
		if parts[3] == "threads" {
			text, markup = a.sessionBrowserThreadView(token, browser)
		} else {
			text, markup = a.sessionBrowserProjectView(token, browser)
		}
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: query.Message.Message.Chat.ID, MessageID: query.Message.Message.ID, Text: text, ReplyMarkup: markup})
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	case "back":
		browser.projectIndex = -1
		browser.page = 0
		text, markup := a.sessionBrowserProjectView(token, browser)
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: query.Message.Message.Chat.ID, MessageID: query.Message.Message.ID, Text: text, ReplyMarkup: markup})
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	case "refresh":
		threads, err := a.fetchCodexThreads(ctx, browser.query)
		if err != nil {
			_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Refresh failed.", ShowAlert: true})
			return
		}
		browser.threads = threads
		browser.projects = groupSessionProjects(threads, a.projectsSnapshot())
		browser.projectIndex = -1
		browser.page = 0
		text, markup := a.sessionBrowserProjectView(token, browser)
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: query.Message.Message.Chat.ID, MessageID: query.Message.Message.ID, Text: text, ReplyMarkup: markup})
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Refreshed."})
	case "attach":
		if len(parts) != 4 {
			return
		}
		index, err := strconv.Atoi(parts[3])
		if err != nil || index < 0 || index >= len(browser.threads) {
			return
		}
		a.attachSessionFromBrowser(ctx, b, query, token, browser, browser.threads[index])
	}
}

func (a *app) attachSessionFromBrowser(ctx context.Context, b *bot.Bot, query *models.CallbackQuery, token string, browser *sessionBrowser, thread codex.Thread) {
	key := fmt.Sprintf("%d:%d", browser.chatID, browser.messageThreadID)
	if a.activeSession(key) != nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "This topic already has an active Codex session.", ShowAlert: true})
		return
	}
	title := threadTitle(thread)
	if title == "" {
		title = "Codex session"
	}
	conv := state.Conversation{
		ChatID:          browser.chatID,
		MessageThreadID: browser.messageThreadID,
		CodexThreadID:   thread.ID,
		ProjectName:     sessionProjectName(thread.Cwd),
		CWD:             thread.Cwd,
		Projectless:     strings.TrimSpace(thread.Cwd) == "",
		TopicTitle:      topicTitle(sessionProjectName(thread.Cwd), title),
		TopicNamed:      false,
	}
	if err := a.store.Upsert(conv); err != nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Could not save mapping.", ShowAlert: true})
		return
	}
	if conv.TopicTitle != "" {
		if _, err := b.EditForumTopic(ctx, &bot.EditForumTopicParams{
			ChatID:          browser.chatID,
			MessageThreadID: browser.messageThreadID,
			Name:            conv.TopicTitle,
		}); err != nil {
			log.Printf("rename attached session topic: %v", err)
		} else {
			conv.TopicNamed = true
			_ = a.store.Upsert(conv)
		}
	}
	a.forgetSessionBrowser(token)
	if query.Message.Message != nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    query.Message.Message.Chat.ID,
			MessageID: query.Message.Message.ID,
			Text:      fmt.Sprintf("Attached Codex session: %s\nSyncing recent history...", title),
		})
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Session attached."})
	syncText := "Synced recent history."
	if err := a.syncRecentAttachedHistory(ctx, b, &conv); err != nil {
		log.Printf("sync attached session history: %v", err)
		syncText = "Recent history sync failed; live Desktop replies will still be mirrored from now on."
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:              browser.chatID,
			MessageThreadID:     browser.messageThreadID,
			Text:                "Attached, but recent history sync failed:\n\n" + err.Error(),
			DisableNotification: true,
		})
	}
	if query.Message.Message != nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    query.Message.Message.Chat.ID,
			MessageID: query.Message.Message.ID,
			Text:      fmt.Sprintf("Attached Codex session: %s\n%s Live Desktop replies will be mirrored here.", title, syncText),
		})
	}
	a.requestMirrorRefresh()
	if browser.pendingFreshKey == "" {
		return
	}
	pending, ok := a.takeFreshTopic(browser.pendingFreshKey)
	if !ok {
		return
	}
	if name, ok := pendingFreshTopicCommandName(pending); ok {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:              pending.chatID,
			MessageThreadID:     pending.messageThreadID,
			Text:                fmt.Sprintf("Not submitting /%s to Codex because it is a Telegram command.", name),
			DisableNotification: true,
		})
		return
	}
	a.submitBuiltPrompt(ctx, b, pending.chatID, pending.messageThreadID, pending.replyMessageID, pending.input, pending.displayText, pending.collaborationMode)
}
