package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"dexgram/internal/codex"

	tgmd "github.com/eekstunt/telegramify-markdown-go"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	draftMinInterval  = 3 * time.Second
	runLogMinInterval = 2 * time.Second
)

func sendRichMessage(ctx context.Context, b *bot.Bot, chatID int64, messageThreadID int, text string) error {
	return sendRichMessageNotify(ctx, b, chatID, messageThreadID, text, true)
}

func sendRichMessageNotify(ctx context.Context, b *bot.Bot, chatID int64, messageThreadID int, text string, notify bool) error {
	for _, message := range renderTelegramMessages(text, 3200) {
		if err := waitTelegramQueue(ctx, "send rich message", chatID, messageThreadID); err != nil {
			return err
		}
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:              chatID,
			MessageThreadID:     messageThreadID,
			Text:                message.Text,
			Entities:            message.Entities,
			DisableNotification: !notify,
		})
		if err != nil {
			if _, ok := logTelegramPressure("send rich message", chatID, messageThreadID, err); !ok {
				log.Printf("send rich message chat_id=%d thread_id=%d entities=%d: %v", chatID, messageThreadID, len(message.Entities), err)
			}
			if err := waitTelegramQueue(ctx, "send rich fallback", chatID, messageThreadID); err != nil {
				return err
			}
			_, fallbackErr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:              chatID,
				MessageThreadID:     messageThreadID,
				Text:                message.Text,
				DisableNotification: !notify,
			})
			if fallbackErr != nil {
				logTelegramPressure("send rich fallback", chatID, messageThreadID, fallbackErr)
				return fallbackErr
			}
		}
	}
	return nil
}

func editRichMessage(ctx context.Context, b *bot.Bot, chatID int64, messageID int, text string) error {
	if len([]rune(text)) > 3200 {
		return fmt.Errorf("message too long to edit safely")
	}
	rendered := firstRenderedTelegramMessage(text, 3200)
	if err := waitTelegramQueue(ctx, "edit rich message", chatID, 0); err != nil {
		return err
	}
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      rendered.Text,
		Entities:  rendered.Entities,
	})
	if err == nil {
		return nil
	}
	logTelegramPressure("edit rich message", chatID, 0, err)
	if err := waitTelegramQueue(ctx, "edit rich fallback", chatID, 0); err != nil {
		return err
	}
	_, fallbackErr := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
	})
	if fallbackErr != nil {
		logTelegramPressure("edit rich fallback", chatID, 0, fallbackErr)
		return fallbackErr
	}
	return err
}

type runLog struct {
	ctx             context.Context
	bot             *bot.Bot
	chatID          int64
	messageThreadID int
	messageID       int
	lines           []runLogEntry
	byID            map[string]int
	lastFlush       time.Time
	lastRendered    string
}

type liveTextMessage struct {
	ctx             context.Context
	bot             *bot.Bot
	chatID          int64
	messageThreadID int
	messageID       int
	draftID         string
	draftOK         bool
	draftFailed     bool
	lastDraftAt     time.Time
	nextDraftAt     time.Time
	text            string
}

type runLogEntry struct {
	id   string
	line string
}

func (t *telegramTurn) ensureInitial(ctx context.Context, b *bot.Bot) {
	if t.Initial != nil {
		return
	}
	t.Initial = &liveTextMessage{
		ctx:             ctx,
		bot:             b,
		chatID:          t.ChatID,
		messageThreadID: t.MessageThreadID,
	}
}

func (t *telegramTurn) ensureRunLog(ctx context.Context, b *bot.Bot) {
	if t.RunLog != nil {
		return
	}
	t.RunLog = newRunLog(ctx, b, t.ChatID, t.MessageThreadID)
}

func (m *liveTextMessage) set(text string) {
	m.text = text
	rendered := firstRenderedTelegramMessage(text, 3200)
	if m.messageID == 0 {
		if err := waitTelegramQueue(m.ctx, "send live text", m.chatID, m.messageThreadID); err != nil {
			return
		}
		msg, err := m.bot.SendMessage(m.ctx, &bot.SendMessageParams{
			ChatID:              m.chatID,
			MessageThreadID:     m.messageThreadID,
			Text:                rendered.Text,
			Entities:            rendered.Entities,
			DisableNotification: true,
		})
		if err != nil {
			logTelegramPressure("send live text", m.chatID, m.messageThreadID, err)
			log.Printf("send live text: %v", err)
			return
		}
		m.messageID = msg.ID
		return
	}
	if err := editRichMessage(m.ctx, m.bot, m.chatID, m.messageID, text); err != nil {
		log.Printf("edit live text: %v", err)
	}
}

func (m *liveTextMessage) draft(text string) {
	if m.messageID != 0 || m.draftFailed {
		return
	}
	now := time.Now()
	if !m.nextDraftAt.IsZero() && now.Before(m.nextDraftAt) {
		return
	}
	if !m.lastDraftAt.IsZero() && now.Sub(m.lastDraftAt) < draftMinInterval {
		return
	}
	if m.draftID == "" {
		m.draftID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if err := waitTelegramQueue(m.ctx, "send draft", m.chatID, m.messageThreadID); err != nil {
		return
	}
	_, err := m.bot.SendMessageDraft(m.ctx, &bot.SendMessageDraftParams{
		ChatID:          m.chatID,
		MessageThreadID: m.messageThreadID,
		DraftID:         m.draftID,
		Text:            text,
	})
	if err != nil {
		if retryAfter, ok := logTelegramPressure("send draft", m.chatID, m.messageThreadID, err); ok {
			m.nextDraftAt = time.Now().Add(retryAfter + time.Second)
			return
		}
		m.draftFailed = true
		log.Printf("send draft: %v", err)
		return
	}
	m.lastDraftAt = now
	m.draftOK = true
}

func (m *liveTextMessage) delete() {
	if m == nil || m.messageID == 0 {
		return
	}
	if err := waitTelegramQueue(m.ctx, "delete live text", m.chatID, m.messageThreadID); err != nil {
		return
	}
	if _, err := m.bot.DeleteMessage(m.ctx, &bot.DeleteMessageParams{
		ChatID:    m.chatID,
		MessageID: m.messageID,
	}); err != nil {
		logTelegramPressure("delete live text", m.chatID, m.messageThreadID, err)
		log.Printf("delete live text: %v", err)
		return
	}
	m.messageID = 0
}

func newRunLog(ctx context.Context, b *bot.Bot, chatID int64, messageThreadID int) *runLog {
	return &runLog{
		ctx:             ctx,
		bot:             b,
		chatID:          chatID,
		messageThreadID: messageThreadID,
		byID:            map[string]int{},
	}
}

func (r *runLog) start(item codex.ThreadItem) {
	line := runLogLine(item)
	if line == "" {
		return
	}
	if item.ID != "" {
		r.byID[item.ID] = len(r.lines)
	}
	r.lines = append(r.lines, runLogEntry{id: item.ID, line: line})
	r.flush()
}

func (r *runLog) output(itemID, delta string) {
	// The live run log is an activity list, not a terminal transcript. Codex
	// streams command output separately, but Telegram becomes noisy fast if we
	// append it here.
}

func (r *runLog) complete(item codex.ThreadItem) {
	line := runLogLine(item)
	if line == "" {
		return
	}
	if item.ID != "" {
		if i, ok := r.byID[item.ID]; ok && i >= 0 && i < len(r.lines) {
			r.lines[i].line = line
			r.flush()
			return
		}
		r.byID[item.ID] = len(r.lines)
	}
	r.lines = append(r.lines, runLogEntry{id: item.ID, line: line})
	r.flush()
}

func (r *runLog) finish() {
	if r.messageID == 0 {
		return
	}
	if err := waitTelegramQueue(r.ctx, "delete run log", r.chatID, r.messageThreadID); err != nil {
		return
	}
	if _, err := r.bot.DeleteMessage(r.ctx, &bot.DeleteMessageParams{
		ChatID:    r.chatID,
		MessageID: r.messageID,
	}); err != nil {
		logTelegramPressure("delete run log", r.chatID, r.messageThreadID, err)
		log.Printf("delete run log: %v", err)
	}
}

func (r *runLog) flush() {
	now := time.Now()
	if r.messageID != 0 && !r.lastFlush.IsZero() && now.Sub(r.lastFlush) < runLogMinInterval {
		return
	}
	lines := make([]string, 0, len(r.lines))
	for _, entry := range r.lines {
		lines = append(lines, entry.line)
	}
	text := "Run log\n\n" + strings.Join(lines, "\n")
	if len([]rune(text)) > 3500 {
		text = truncateRunLog(text, 3500)
	}
	rendered := "```text\n" + text + "\n```"
	message := firstRenderedTelegramMessage(rendered, 3900)
	renderHash := telegramMessageHash(message)
	if renderHash == r.lastRendered {
		return
	}
	if r.messageID == 0 {
		if err := waitTelegramQueue(r.ctx, "send run log", r.chatID, r.messageThreadID); err != nil {
			return
		}
		msg, err := r.bot.SendMessage(r.ctx, &bot.SendMessageParams{
			ChatID:              r.chatID,
			MessageThreadID:     r.messageThreadID,
			Text:                message.Text,
			Entities:            message.Entities,
			DisableNotification: true,
		})
		if err != nil {
			logTelegramPressure("send run log", r.chatID, r.messageThreadID, err)
			log.Printf("send run log: %v", err)
			return
		}
		r.messageID = msg.ID
		r.lastRendered = renderHash
		r.lastFlush = now
		return
	}
	if err := waitTelegramQueue(r.ctx, "edit run log", r.chatID, r.messageThreadID); err != nil {
		return
	}
	_, err := r.bot.EditMessageText(r.ctx, &bot.EditMessageTextParams{
		ChatID:    r.chatID,
		MessageID: r.messageID,
		Text:      message.Text,
		Entities:  message.Entities,
	})
	if err != nil {
		if isTelegramNoopEdit(err) {
			r.lastRendered = renderHash
			return
		}
		logTelegramPressure("edit run log", r.chatID, r.messageThreadID, err)
		log.Printf("edit run log: %v", err)
		return
	}
	r.lastRendered = renderHash
	r.lastFlush = now
}

func logTelegramPressure(op string, chatID int64, messageThreadID int, err error) (time.Duration, bool) {
	var rateErr *bot.TooManyRequestsError
	if !errors.As(err, &rateErr) {
		return 0, false
	}
	retryAfter := time.Duration(rateErr.RetryAfter) * time.Second
	backoffTelegramQueue(retryAfter + time.Second)
	log.Printf("telegram pressure op=%q chat_id=%d thread_id=%d retry_after=%s reason=%q", op, chatID, messageThreadID, retryAfter, rateErr.Message)
	return retryAfter, true
}

func isTelegramNoopEdit(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "message is not modified")
}

func sameTelegramText(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}

func isRunLogItem(item codex.ThreadItem) bool {
	switch item.Type {
	case "commandExecution", "fileChange", "mcpToolCall", "dynamicToolCall", "webSearch", "imageView", "imageGeneration", "collabAgentToolCall":
		return true
	default:
		return false
	}
}

func runLogLine(item codex.ThreadItem) string {
	switch item.Type {
	case "commandExecution":
		return "shell: " + trimCommandForLog(item.Command, 180)
	case "fileChange":
		parts := make([]string, 0, len(item.Changes))
		for _, change := range item.Changes {
			if strings.TrimSpace(change.Path) != "" {
				parts = append(parts, compactPath(change.Path, 80))
			}
		}
		return "edit: " + oneLine(strings.Join(parts, ", "), 180)
	case "mcpToolCall", "dynamicToolCall", "collabAgentToolCall":
		return "tool: " + strings.TrimSpace(strings.TrimSpace(item.Server)+" "+strings.TrimSpace(item.Tool))
	case "webSearch":
		return "web: " + oneLine(item.Query, 180)
	case "imageView":
		return "image: " + compactPath(item.Path, 180)
	case "imageGeneration":
		path := item.SavedPath
		if path == "" {
			path = item.Result
		}
		return "image-gen: " + compactPath(path, 180)
	default:
		return ""
	}
}

func trimCommandForLog(command string, max int) string {
	command = strings.TrimSpace(command)
	lower := strings.ToLower(command)
	for _, marker := range []string{" -command ", " -c "} {
		if idx := strings.Index(lower, marker); idx >= 0 {
			command = strings.TrimSpace(command[idx+len(marker):])
			break
		}
	}
	command = trimOuterQuotes(command)
	command = strings.TrimPrefix(command, "& ")
	command = trimOuterQuotes(command)
	return oneLine(command, max)
}

func trimOuterQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return s
	}
	first := s[0]
	last := s[len(s)-1]
	if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

func compactPath(path string, max int) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return oneLine(path, max)
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, `\\`, `\`)
	if len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	if max <= 3 {
		return string(r[:max])
	}
	head := (max - 5) / 2
	tail := max - 5 - head
	return string(r[:head]) + " ... " + string(r[len(r)-tail:])
}

func truncateRunLog(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return "...\n" + string(r[len(r)-max+4:])
}

func splitTelegramChunks(text string, max int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{" "}
	}
	var chunks []string
	var current strings.Builder
	for _, para := range strings.Split(text, "\n\n") {
		add := para
		if current.Len() > 0 {
			add = "\n\n" + para
		}
		if len([]rune(current.String()+add)) <= max {
			current.WriteString(add)
			continue
		}
		if current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		for len([]rune(para)) > max {
			r := []rune(para)
			cut := max
			for cut > max/2 && r[cut-1] != '\n' && r[cut-1] != ' ' {
				cut--
			}
			if cut <= max/2 {
				cut = max
			}
			chunks = append(chunks, strings.TrimSpace(string(r[:cut])))
			para = strings.TrimSpace(string(r[cut:]))
		}
		current.WriteString(para)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

type renderedTelegramMessage struct {
	Text     string
	Entities []models.MessageEntity
}

func renderTelegramMessages(markdown string, max int) []renderedTelegramMessage {
	if max <= 0 {
		max = 4096
	}
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return []renderedTelegramMessage{{Text: " "}}
	}
	converted := tgmd.Convert(markdown)
	chunks := tgmd.Split(tgmd.Message{
		Text:     converted.Text,
		Entities: converted.Entities,
	}, max)
	out := make([]renderedTelegramMessage, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, renderedTelegramMessage{
			Text:     chunk.Text,
			Entities: convertTelegramEntities(chunk.Entities),
		})
	}
	if len(out) == 0 {
		return []renderedTelegramMessage{{Text: " "}}
	}
	return out
}

func firstRenderedTelegramMessage(markdown string, max int) renderedTelegramMessage {
	messages := renderTelegramMessages(markdown, max)
	if len(messages) == 0 {
		return renderedTelegramMessage{Text: " "}
	}
	return messages[0]
}

func convertTelegramEntities(entities []tgmd.Entity) []models.MessageEntity {
	out := make([]models.MessageEntity, 0, len(entities))
	for _, entity := range entities {
		if entity.Type == "text_link" && !validTelegramTextLinkURL(entity.URL) {
			continue
		}
		out = append(out, models.MessageEntity{
			Type:     models.MessageEntityType(entity.Type),
			Offset:   entity.Offset,
			Length:   entity.Length,
			URL:      entity.URL,
			Language: entity.Language,
		})
	}
	return out
}

func validTelegramTextLinkURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "tg":
		return true
	default:
		return false
	}
}

func telegramMessageHash(message renderedTelegramMessage) string {
	parts := []string{message.Text}
	for _, entity := range message.Entities {
		parts = append(parts, fmt.Sprintf("%s:%d:%d:%s:%s", entity.Type, entity.Offset, entity.Length, entity.URL, entity.Language))
	}
	return strings.Join(parts, "\x00")
}
