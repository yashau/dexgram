package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"dexgram/internal/codex"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func sendRichMessage(ctx context.Context, b *bot.Bot, chatID int64, messageThreadID int, text string) error {
	for _, chunk := range splitTelegramChunks(text, 3200) {
		formatted := renderTelegramMarkdown(chunk)
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: messageThreadID,
			Text:            formatted,
			ParseMode:       models.ParseModeMarkdown,
		})
		if err != nil {
			_, fallbackErr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:          chatID,
				MessageThreadID: messageThreadID,
				Text:            chunk,
			})
			if fallbackErr != nil {
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
	formatted := renderTelegramMarkdown(text)
	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      formatted,
		ParseMode: models.ParseModeMarkdown,
	})
	if err == nil {
		return nil
	}
	_, fallbackErr := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
	})
	if fallbackErr != nil {
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
	if m.messageID == 0 {
		msg, err := m.bot.SendMessage(m.ctx, &bot.SendMessageParams{
			ChatID:          m.chatID,
			MessageThreadID: m.messageThreadID,
			Text:            renderTelegramMarkdown(text),
			ParseMode:       models.ParseModeMarkdown,
		})
		if err != nil {
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
	if m.draftID == "" {
		m.draftID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	_, err := m.bot.SendMessageDraft(m.ctx, &bot.SendMessageDraftParams{
		ChatID:          m.chatID,
		MessageThreadID: m.messageThreadID,
		DraftID:         m.draftID,
		Text:            text,
	})
	if err != nil {
		m.draftFailed = true
		log.Printf("send draft: %v", err)
		return
	}
	m.draftOK = true
}

func (m *liveTextMessage) delete() {
	if m == nil || m.messageID == 0 {
		return
	}
	if _, err := m.bot.DeleteMessage(m.ctx, &bot.DeleteMessageParams{
		ChatID:    m.chatID,
		MessageID: m.messageID,
	}); err != nil {
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
	if _, err := r.bot.DeleteMessage(r.ctx, &bot.DeleteMessageParams{
		ChatID:    r.chatID,
		MessageID: r.messageID,
	}); err != nil {
		log.Printf("delete run log: %v", err)
	}
}

func (r *runLog) flush() {
	r.lastFlush = time.Now()
	lines := make([]string, 0, len(r.lines))
	for _, entry := range r.lines {
		lines = append(lines, entry.line)
	}
	text := "Run log\n\n" + strings.Join(lines, "\n")
	if len([]rune(text)) > 3500 {
		text = truncateRunLog(text, 3500)
	}
	rendered := "```text\n" + escapeCode(text) + "\n```"
	if rendered == r.lastRendered {
		return
	}
	if r.messageID == 0 {
		msg, err := r.bot.SendMessage(r.ctx, &bot.SendMessageParams{
			ChatID:          r.chatID,
			MessageThreadID: r.messageThreadID,
			Text:            rendered,
			ParseMode:       models.ParseModeMarkdown,
		})
		if err != nil {
			log.Printf("send run log: %v", err)
			return
		}
		r.messageID = msg.ID
		r.lastRendered = rendered
		return
	}
	_, err := r.bot.EditMessageText(r.ctx, &bot.EditMessageTextParams{
		ChatID:    r.chatID,
		MessageID: r.messageID,
		Text:      rendered,
		ParseMode: models.ParseModeMarkdown,
	})
	if err != nil {
		if isTelegramNoopEdit(err) {
			r.lastRendered = rendered
			return
		}
		log.Printf("edit run log: %v", err)
		return
	}
	r.lastRendered = rendered
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

func renderTelegramMarkdown(md string) string {
	lines := strings.Split(md, "\n")
	var out strings.Builder
	inFence := false
	fenceLang := ""
	var code []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inFence {
				if out.Len() > 0 {
					out.WriteByte('\n')
				}
				out.WriteString("```")
				if fenceLang != "" {
					out.WriteString(escapeCode(fenceLang))
				}
				out.WriteByte('\n')
				out.WriteString(escapeCode(strings.Join(code, "\n")))
				out.WriteString("\n```")
				inFence = false
				fenceLang = ""
				code = nil
			} else {
				inFence = true
				fenceLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
				code = nil
			}
			continue
		}
		if inFence {
			code = append(code, line)
			continue
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(renderTelegramLine(line))
	}
	if inFence {
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString("```")
		if fenceLang != "" {
			out.WriteString(escapeCode(fenceLang))
		}
		out.WriteByte('\n')
		out.WriteString(escapeCode(strings.Join(code, "\n")))
		out.WriteString("\n```")
	}
	return out.String()
}

func renderTelegramLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "> ") {
		return "> " + renderInlineMarkdown(strings.TrimSpace(strings.TrimPrefix(trimmed, "> ")))
	}
	if strings.HasPrefix(trimmed, "### ") {
		return "*" + escapeMarkdownV2(strings.TrimSpace(strings.TrimPrefix(trimmed, "### "))) + "*"
	}
	if strings.HasPrefix(trimmed, "## ") {
		return "*" + escapeMarkdownV2(strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))) + "*"
	}
	if strings.HasPrefix(trimmed, "# ") {
		return "*" + escapeMarkdownV2(strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))) + "*"
	}
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		return "• " + renderInlineMarkdown(strings.TrimSpace(trimmed[2:]))
	}
	if i := orderedBulletIndex(trimmed); i > 0 {
		return escapeMarkdownV2(trimmed[:i]) + " " + renderInlineMarkdown(strings.TrimSpace(trimmed[i+1:]))
	}
	return renderInlineMarkdown(line)
}

func renderInlineMarkdown(s string) string {
	var out strings.Builder
	for len(s) > 0 {
		start, marker := nextInlineMarker(s)
		if start < 0 {
			out.WriteString(escapeMarkdownV2(s))
			break
		}
		out.WriteString(escapeMarkdownV2(s[:start]))
		switch marker {
		case "`":
			rest := s[start+1:]
			end := strings.Index(rest, "`")
			if end < 0 {
				out.WriteString("\\`")
				out.WriteString(escapeMarkdownV2(rest))
				return out.String()
			}
			out.WriteByte('`')
			out.WriteString(escapeCode(rest[:end]))
			out.WriteByte('`')
			s = rest[end+1:]
		case "[":
			label, url, ok, advance := parseMarkdownLink(s[start:])
			if !ok {
				out.WriteString("\\[")
				s = s[start+1:]
				continue
			}
			out.WriteByte('[')
			out.WriteString(escapeMarkdownV2(label))
			out.WriteString("](")
			out.WriteString(escapeURL(url))
			out.WriteByte(')')
			s = s[start+advance:]
		case "**":
			rest := s[start+2:]
			end := strings.Index(rest, "**")
			if end < 0 {
				out.WriteString("\\*\\*")
				out.WriteString(escapeMarkdownV2(rest))
				return out.String()
			}
			out.WriteByte('*')
			out.WriteString(renderInlineMarkdown(rest[:end]))
			out.WriteByte('*')
			s = rest[end+2:]
		default:
			out.WriteString(escapeMarkdownV2(marker))
			s = s[start+len(marker):]
		}
	}
	return out.String()
}

func nextInlineMarker(s string) (int, string) {
	best := -1
	marker := ""
	for _, candidate := range []string{"`", "[", "**"} {
		i := strings.Index(s, candidate)
		if i >= 0 && (best < 0 || i < best) {
			best = i
			marker = candidate
		}
	}
	return best, marker
}

func parseMarkdownLink(s string) (string, string, bool, int) {
	closeLabel := strings.Index(s, "](")
	if closeLabel < 1 {
		return "", "", false, 0
	}
	closeURL := strings.Index(s[closeLabel+2:], ")")
	if closeURL < 1 {
		return "", "", false, 0
	}
	closeURL += closeLabel + 2
	label := s[1:closeLabel]
	url := s[closeLabel+2 : closeURL]
	if strings.TrimSpace(label) == "" || strings.TrimSpace(url) == "" {
		return "", "", false, 0
	}
	return label, url, true, closeURL + 1
}

func orderedBulletIndex(s string) int {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(s) || s[i] != '.' || s[i+1] != ' ' {
		return -1
	}
	return i
}

func escapeURL(s string) string {
	replacer := strings.NewReplacer("\\", "\\\\", ")", "\\)")
	return replacer.Replace(s)
}

func escapeMarkdownV2(s string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(s)
}

func escapeCode(s string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "`", "\\`")
	return replacer.Replace(s)
}
