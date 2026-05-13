package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"dexgram/internal/codexprojects"
	"dexgram/internal/state"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (a *app) handleUpdateFatal(ctx context.Context, b *bot.Bot, update *models.Update) {
	defer func() {
		if recovered := recover(); recovered != nil {
			reportFatalPanic(recovered)
			os.Exit(2)
		}
	}()
	a.handleUpdate(ctx, b, update)
}

func (a *app) handleUpdate(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery != nil {
		a.handleCallback(ctx, b, update.CallbackQuery)
		return
	}
	msg := update.Message
	if msg == nil {
		return
	}
	if !a.allowedChat(msg.Chat.ID) {
		log.Printf("ignored message from chat_id=%d thread_id=%d", msg.Chat.ID, msg.MessageThreadID)
		return
	}
	ackTelegramMessage(ctx, b, msg)

	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	if text == "" {
		text = "<non-text message>"
	}
	log.Printf("telegram message chat_id=%d thread_id=%d message_id=%d text=%q",
		msg.Chat.ID,
		msg.MessageThreadID,
		msg.ID,
		text,
	)

	if a.handlePendingInputReply(ctx, b, msg) {
		return
	}

	commandText := strings.TrimSpace(msg.Text)
	commandName, commandArg, isCommand := parseTelegramCommand(commandText)
	if isCommand && commandName == "project" {
		a.handleProjectCommand(ctx, b, msg, text)
		return
	}
	if isCommand && commandName == "new" {
		a.handleNewCommand(ctx, b, msg, commandText)
		return
	}
	if isCommand && commandName == "status" {
		a.handleStatusCommand(ctx, b, msg)
		return
	}
	if isCommand && commandName == "sync" {
		goFatal(func() {
			a.handleSyncCommand(ctx, b, msg)
		})
		return
	}
	if isCommand && commandName == "update" {
		goFatal(func() {
			a.handleUpdateCommand(ctx, b, msg)
		})
		return
	}
	if isCommand && (commandName == "stop" || commandName == "cancel") {
		a.handleStopCommand(ctx, b, msg)
		return
	}
	if isCommand && commandName == "steer" {
		a.handleSteerCommand(ctx, b, msg, commandText)
		return
	}
	if isCommand && commandName == "goal" {
		goFatal(func() {
			a.handleGoalCommand(ctx, b, msg, commandArg)
		})
		return
	}
	if isCommand && commandName == "plan" {
		if strings.TrimSpace(commandArg) == "" {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:              msg.Chat.ID,
				MessageThreadID:     msg.MessageThreadID,
				Text:                "Usage: /plan <message>",
				DisableNotification: true,
			})
			return
		}
		goFatal(func() {
			a.handlePlanPrompt(ctx, b, msg, commandArg)
		})
		return
	}
	if isCommand && commandName == "settings" {
		a.handleSettingsCommand(ctx, b, msg)
		return
	}
	if isCommand && commandName == "model" {
		goFatal(func() {
			a.handleModelCommand(ctx, b, msg, commandArg)
		})
		return
	}
	if isCommand && (commandName == "effort" || commandName == "reasoning") {
		a.handleEffortCommand(ctx, b, msg, commandArg)
		return
	}
	prompt := strings.TrimSpace(msg.Text)
	if prompt == "" {
		prompt = strings.TrimSpace(msg.Caption)
	}
	if prompt == "" {
		if messageHasAttachment(msg) {
			goFatal(func() {
				a.handleStageAttachments(ctx, b, msg)
			})
		}
		return
	}
	goFatal(func() {
		a.handlePrompt(ctx, b, msg, prompt)
	})
}

func (a *app) allowedChat(chatID int64) bool {
	return a.cfg.Telegram.ChatID == 0 || chatID == a.cfg.Telegram.ChatID
}

func (a *app) handleStageAttachments(ctx context.Context, b *bot.Bot, msg *models.Message) {
	count, err := a.stageMessageAttachments(ctx, b, msg)
	if err != nil {
		log.Printf("stage attachment failed: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Dexgram could not stage that attachment:\n\n" + err.Error(),
			ReplyParameters: &models.ReplyParameters{
				MessageID:                msg.ID,
				ChatID:                   msg.Chat.ID,
				AllowSendingWithoutReply: true,
			},
		})
		return
	}
	if count == 0 {
		return
	}
	text := fmt.Sprintf("Staged %d attachment. Send a message here and I will include it with the Codex prompt.", count)
	if count != 1 {
		text = fmt.Sprintf("Staged %d attachments. Send a message here and I will include them with the Codex prompt.", count)
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            text,
		ReplyParameters: &models.ReplyParameters{
			MessageID:                msg.ID,
			ChatID:                   msg.Chat.ID,
			AllowSendingWithoutReply: true,
		},
	})
}

func parseTelegramCommand(text string) (string, string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	token := text
	rest := ""
	if fields := strings.Fields(text); len(fields) > 0 {
		token = fields[0]
		if len(text) > len(token) {
			rest = text[len(token):]
		}
	}
	name := strings.TrimPrefix(token, "/")
	if at := strings.Index(name, "@"); at >= 0 {
		name = name[:at]
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", "", false
	}
	return name, strings.TrimSpace(rest), true
}

func (a *app) handlePendingInputReply(ctx context.Context, b *bot.Bot, msg *models.Message) bool {
	if msg.ReplyToMessage == nil {
		return false
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		text = strings.TrimSpace(msg.Caption)
	}
	if text == "" {
		return false
	}
	a.mu.Lock()
	var token string
	var pending *pendingInput
	for candidate, input := range a.inputs {
		if input.chatID == msg.Chat.ID &&
			input.messageThreadID == msg.MessageThreadID &&
			input.promptMessageID == msg.ReplyToMessage.ID {
			token = candidate
			pending = input
			break
		}
	}
	a.mu.Unlock()
	if pending == nil {
		return false
	}
	lines := splitNonEmptyLines(text)
	answers := map[string]any{}
	for i, q := range pending.questions {
		answer := text
		if i < len(lines) {
			answer = lines[i]
		}
		answers[q.ID] = map[string]any{"answers": []string{answer}}
	}
	pending.ch <- inputDecision{result: answers}
	a.mu.Lock()
	delete(a.inputs, token)
	a.mu.Unlock()
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            "Answered Codex input request.",
		ReplyParameters: &models.ReplyParameters{
			MessageID:                msg.ID,
			ChatID:                   msg.Chat.ID,
			AllowSendingWithoutReply: true,
		},
	})
	return true
}

func (a *app) handleProjectCommand(ctx context.Context, b *bot.Bot, msg *models.Message, text string) {
	conv, ok, err := a.store.Get(msg.Chat.ID, msg.MessageThreadID)
	if err != nil {
		log.Printf("read conversation for project command: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Could not read Dexgram state: " + err.Error(),
		})
		return
	}
	if ok && conv.CodexThreadID != "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "This chat has already started in Codex, so it cannot be moved to a project.",
		})
		return
	}

	fields := strings.Fields(text)
	query := strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
	if query == "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Usage: /project <project name>",
		})
		return
	}

	projects, err := a.refreshProjects()
	if err != nil {
		log.Printf("refresh Codex projects: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Could not refresh Codex projects: " + err.Error(),
		})
		return
	}

	matches := codexprojects.Match(projects, query, 6)
	if len(matches) == 0 {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "No Codex project matched: " + query,
		})
		return
	}
	if len(matches) == 1 {
		a.setTopicProject(ctx, b, msg.Chat.ID, msg.MessageThreadID, matches[0], true)
		return
	}

	rows := make([][]models.InlineKeyboardButton, 0, len(matches))
	for _, project := range matches {
		index := projectIndex(projects, project)
		if index < 0 {
			continue
		}
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         project.Name,
			CallbackData: "project:" + strconv.Itoa(msg.MessageThreadID) + ":" + strconv.Itoa(index),
		}})
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            "Select the Codex project for this chat:",
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: rows,
		},
	})
}

func (a *app) handleCallback(ctx context.Context, b *bot.Bot, query *models.CallbackQuery) {
	if strings.HasPrefix(query.Data, "ap:") {
		a.handleApprovalCallback(ctx, b, query)
		return
	}
	if strings.HasPrefix(query.Data, "st:") {
		a.handleSteerCallback(ctx, b, query)
		return
	}
	if strings.HasPrefix(query.Data, "dq:") {
		a.handleDeleteQueuedCallback(ctx, b, query)
		return
	}
	if strings.HasPrefix(query.Data, "sp:") {
		a.handleStopCallback(ctx, b, query)
		return
	}
	if strings.HasPrefix(query.Data, "ui:") {
		a.handleUserInputCallback(ctx, b, query)
		return
	}
	if strings.HasPrefix(query.Data, "settings:") {
		a.handleSettingsCallback(ctx, b, query)
		return
	}
	if strings.HasPrefix(query.Data, "model:") {
		a.handleModelCallback(ctx, b, query)
		return
	}
	if strings.HasPrefix(query.Data, "effort:") {
		a.handleEffortCallback(ctx, b, query)
		return
	}
	if !strings.HasPrefix(query.Data, "project:") {
		return
	}
	parts := strings.SplitN(query.Data, ":", 3)
	if len(parts) != 3 {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Invalid project selection.",
			ShowAlert:       true,
		})
		return
	}
	threadID, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	chatID := query.Message.Message.Chat.ID
	if !a.allowedChat(chatID) {
		return
	}
	conv, ok, err := a.store.Get(chatID, threadID)
	if err != nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Could not read Dexgram state.",
			ShowAlert:       true,
		})
		return
	}
	if ok && conv.CodexThreadID != "" {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "This chat has already started in Codex.",
			ShowAlert:       true,
		})
		return
	}
	projectIndex, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	project, ok := a.projectByIndex(projectIndex)
	if !ok {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Project is no longer available.",
			ShowAlert:       true,
		})
		return
	}
	a.setTopicProject(ctx, b, chatID, threadID, project, false)
	if query.Message.Message != nil {
		_, _ = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:    chatID,
			MessageID: query.Message.Message.ID,
		})
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            "Project set: " + project.Name,
	})
}

func (a *app) handleApprovalCallback(ctx context.Context, b *bot.Bot, query *models.CallbackQuery) {
	parts := strings.Split(query.Data, ":")
	if len(parts) != 3 {
		return
	}
	token := parts[1]
	action := parts[2]
	a.mu.Lock()
	pending := a.approvals[token]
	delete(a.approvals, token)
	a.mu.Unlock()
	if pending == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Approval is no longer pending.",
			ShowAlert:       true,
		})
		return
	}

	var decision approvalDecision
	switch action {
	case "a":
		decision.result = map[string]any{"decision": "accept"}
	case "s":
		decision.result = map[string]any{"decision": "acceptForSession"}
	case "m":
		decision.result = "amend"
	case "pt":
		decision.result = "permission-turn"
	case "ps":
		decision.result = "permission-session"
	case "d":
		decision.result = map[string]any{"decision": "decline"}
	default:
		decision.result = map[string]any{"decision": "cancel"}
	}
	pending.ch <- decision

	text := "Approval resolved."
	if query.Message.Message != nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    query.Message.Message.Chat.ID,
			MessageID: query.Message.Message.ID,
			Text:      text,
		})
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            text,
	})
}

func (a *app) handleUserInputCallback(ctx context.Context, b *bot.Bot, query *models.CallbackQuery) {
	parts := strings.Split(query.Data, ":")
	if len(parts) != 3 {
		return
	}
	token := parts[1]
	a.mu.Lock()
	pending := a.inputs[token]
	a.mu.Unlock()
	if pending == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Input request is no longer pending.", ShowAlert: true})
		return
	}
	if parts[2] == "cancel" {
		pending.ch <- inputDecision{err: fmt.Errorf("user cancelled input request")}
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Cancelled."})
		return
	}
	index, err := strconv.Atoi(parts[2])
	if err != nil || len(pending.questions) != 1 || index < 0 || index >= len(pending.questions[0].Options) {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Invalid answer.", ShowAlert: true})
		return
	}
	q := pending.questions[0]
	answer := q.Options[index].Label
	pending.ch <- inputDecision{result: map[string]any{q.ID: map[string]any{"answers": []string{answer}}}}
	if query.Message.Message != nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    query.Message.Message.Chat.ID,
			MessageID: query.Message.Message.ID,
			Text:      "Answered: " + answer,
		})
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Answered."})
}

func (a *app) handleSteerCallback(ctx context.Context, b *bot.Bot, query *models.CallbackQuery) {
	token := strings.TrimPrefix(query.Data, "st:")
	action, ok := a.turnAction(token)
	if !ok {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "That queued message is no longer available.", ShowAlert: true})
		return
	}
	session := a.activeSession(action.Key)
	if session == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "No active Codex turn.", ShowAlert: true})
		return
	}
	queued := a.sessionTurn(action.Key, action.TurnID)
	activeTurnID := a.currentTurnID(action.Key)
	if queued == nil || !queued.Queued || activeTurnID == "" {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Nothing to steer.", ShowAlert: true})
		return
	}
	if activeTurnID == action.TurnID {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "This turn is already active."})
		return
	}
	if err := steerTurn(ctx, session.client, session.threadID, activeTurnID, queued.Input); err != nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Steer failed: " + err.Error(), ShowAlert: true})
		return
	}
	a.removeSessionTurn(action.Key, action.TurnID)
	a.forgetTurnAction(action.Key, action.TurnID)
	if query.Message.Message != nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    query.Message.Message.Chat.ID,
			MessageID: query.Message.Message.ID,
			Text:      "Steered into the current Codex turn.",
		})
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Steered."})
}

func (a *app) handleDeleteQueuedCallback(ctx context.Context, b *bot.Bot, query *models.CallbackQuery) {
	token := strings.TrimPrefix(query.Data, "dq:")
	action, ok := a.turnAction(token)
	if !ok {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "That queued message is no longer available.", ShowAlert: true})
		return
	}
	session := a.activeSession(action.Key)
	if session == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "No active Codex session.", ShowAlert: true})
		return
	}
	queued := a.sessionTurn(action.Key, action.TurnID)
	if queued == nil || !queued.Queued {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "That queued message is no longer available.", ShowAlert: true})
		return
	}
	a.removeSessionTurn(action.Key, action.TurnID)
	a.forgetTurnAction(action.Key, action.TurnID)
	if query.Message.Message != nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    query.Message.Message.Chat.ID,
			MessageID: query.Message.Message.ID,
			Text:      "Queued Codex message deleted.",
		})
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Deleted."})
}

func (a *app) handleStopCallback(ctx context.Context, b *bot.Bot, query *models.CallbackQuery) {
	token := strings.TrimPrefix(query.Data, "sp:")
	action, ok := a.turnAction(token)
	if !ok {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "That turn is no longer active.", ShowAlert: true})
		return
	}
	session := a.activeSession(action.Key)
	if session == nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "No active Codex session.", ShowAlert: true})
		return
	}
	if err := interruptTurn(ctx, session.client, session.threadID, action.TurnID); err != nil {
		_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Stop failed: " + err.Error(), ShowAlert: true})
		return
	}
	if query.Message.Message != nil {
		_, _ = b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:    query.Message.Message.Chat.ID,
			MessageID: query.Message.Message.ID,
		})
	}
	_, _ = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID, Text: "Stopped."})
}

func (a *app) setTopicProject(ctx context.Context, b *bot.Bot, chatID int64, messageThreadID int, project codexprojects.Project, announce bool) {
	conv, _, err := a.store.Get(chatID, messageThreadID)
	if err != nil {
		log.Printf("read conversation before setting project: %v", err)
	}
	conv.ChatID = chatID
	conv.MessageThreadID = messageThreadID
	conv.ProjectName = project.Name
	conv.CWD = project.Path
	conv.Projectless = false
	conv.TopicNamed = false
	if err := a.store.Upsert(conv); err != nil {
		log.Printf("store topic project: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: messageThreadID,
			Text:            "Project selected, but Dexgram could not save it: " + err.Error(),
		})
		return
	}
	text := "Project set: " + project.Name + "\n" + project.Path + "\n\nNow send the first message to start the Codex chat."
	if announce {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: messageThreadID,
			Text:            text,
		})
	}
}

func (a *app) handleNewCommand(ctx context.Context, b *bot.Bot, msg *models.Message, text string) {
	arg := strings.TrimSpace(strings.TrimPrefix(text, strings.Fields(text)[0]))
	projectName := ""
	cwd := ""
	projectless := true
	title := strings.TrimSpace(arg)
	if strings.Contains(arg, ":") {
		parts := strings.SplitN(arg, ":", 2)
		query := strings.TrimSpace(parts[0])
		title = strings.TrimSpace(parts[1])
		projects, err := a.refreshProjects()
		if err != nil {
			log.Printf("refresh Codex projects: %v", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Could not refresh Codex projects: " + err.Error()})
			return
		}
		matches := codexprojects.Match(projects, query, 2)
		if len(matches) == 1 {
			projectName = matches[0].Name
			cwd = matches[0].Path
			projectless = false
		} else if len(matches) > 1 {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "That project name is ambiguous. Use /project inside the new topic before sending the first message."})
			return
		}
	}
	if title == "" {
		title = "New chat"
	}
	topic, err := b.CreateForumTopic(ctx, &bot.CreateForumTopicParams{
		ChatID: msg.Chat.ID,
		Name:   topicTitle(projectName, title),
	})
	if err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Could not create a Telegram topic: " + err.Error()})
		return
	}
	conv := state.Conversation{
		ChatID:          msg.Chat.ID,
		MessageThreadID: topic.MessageThreadID,
		ProjectName:     projectName,
		CWD:             cwd,
		Projectless:     projectless,
		TopicTitle:      topic.Name,
	}
	if err := a.store.Upsert(conv); err != nil {
		log.Printf("store new topic: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: topic.MessageThreadID,
			Text:            "Topic created, but Dexgram could not save its mapping: " + err.Error(),
		})
		return
	}
	prefix := "New Codex chat ready."
	if projectName != "" {
		prefix = "New Codex chat ready for project: " + projectName
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: topic.MessageThreadID,
		Text:            prefix + "\nSend the first message here.",
	})
}

func (a *app) handleStatusCommand(ctx context.Context, b *bot.Bot, msg *models.Message) {
	key := fmt.Sprintf("%d:%d", msg.Chat.ID, msg.MessageThreadID)
	conv, ok, err := a.store.Get(msg.Chat.ID, msg.MessageThreadID)
	if err != nil {
		log.Printf("read conversation for status: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Could not read Dexgram state: " + err.Error()})
		return
	}
	var parts []string
	if ok {
		parts = append(parts, "Codex thread: "+emptyAs(conv.CodexThreadID, "not started"))
		projectLabel := conv.ProjectName
		if conv.Projectless || projectLabel == "" {
			projectLabel = "none"
		}
		parts = append(parts, "Project: "+projectLabel)
		parts = append(parts, "cwd: "+emptyAs(conv.CWD, "not prepared yet"))
	} else {
		parts = append(parts, "No Dexgram mapping yet.")
	}
	if session := a.activeSession(key); session != nil {
		parts = append(parts, fmt.Sprintf("Active turns known to Dexgram: %d", len(session.turns)))
	} else {
		parts = append(parts, "No active Codex turn.")
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: strings.Join(parts, "\n")})
}

func (a *app) handleStopCommand(ctx context.Context, b *bot.Bot, msg *models.Message) {
	key := fmt.Sprintf("%d:%d", msg.Chat.ID, msg.MessageThreadID)
	session := a.activeSession(key)
	turnID := a.currentTurnID(key)
	if session == nil || turnID == "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "No active Codex turn in this topic."})
		return
	}
	if err := interruptTurn(ctx, session.client, session.threadID, turnID); err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Could not stop Codex: " + err.Error()})
		return
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Stopped the active Codex turn."})
}

func (a *app) handleSteerCommand(ctx context.Context, b *bot.Bot, msg *models.Message, text string) {
	key := fmt.Sprintf("%d:%d", msg.Chat.ID, msg.MessageThreadID)
	session := a.activeSession(key)
	turnID := a.currentTurnID(key)
	steerText := strings.TrimSpace(strings.TrimPrefix(text, strings.Fields(text)[0]))
	if steerText == "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Usage: /steer <message>"})
		return
	}
	if session == nil || turnID == "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "No active Codex turn in this topic."})
		return
	}
	if err := steerTurn(ctx, session.client, session.threadID, turnID, textInput(steerText)); err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Could not steer Codex: " + err.Error()})
		return
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, MessageThreadID: msg.MessageThreadID, Text: "Steered the active Codex turn."})
}

func (a *app) handleGoalCommand(ctx context.Context, b *bot.Bot, msg *models.Message, objective string) {
	objective = strings.TrimSpace(objective)
	if objective == "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Usage: /goal <objective>",
		})
		return
	}
	if err := a.setTopicGoal(ctx, msg.Chat.ID, msg.MessageThreadID, objective); err != nil {
		log.Printf("set codex goal failed: %v", err)
		text := "Dexgram could not set the Codex goal:\n\n" + err.Error()
		if isCodexGoalsDisabledError(err) {
			text = codexGoalsDisabledMessage()
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            text,
		})
		return
	}
	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		MessageThreadID: msg.MessageThreadID,
		Text:            "Codex goal set:\n" + objective,
	})
}

func isCodexGoalsDisabledError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "goals feature is disabled")
}

func codexGoalsDisabledMessage() string {
	return "Codex goals are not enabled yet.\n\n" +
		"To enable /goal, edit:\n" +
		"%USERPROFILE%\\.codex\\config.toml\n\n" +
		"Add this section, or add the goals line under your existing [features] section:\n\n" +
		"[features]\n" +
		"goals = true\n\n" +
		"Then restart Codex/Dexgram and run /goal again."
}
