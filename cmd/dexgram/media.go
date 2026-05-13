package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"dexgram/internal/state"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (a *app) buildTurnInput(ctx context.Context, b *bot.Bot, msg *models.Message, prompt string) ([]map[string]any, string, error) {
	displayText := strings.TrimSpace(prompt)
	var attachmentLines []string
	var input []map[string]any

	staged, err := a.store.ListStagedAttachments(msg.Chat.ID, msg.MessageThreadID)
	if err != nil {
		return nil, "", err
	}
	for _, attachment := range staged {
		input = append(input, codexInputForAttachment(attachment.Path, attachment.Kind))
		attachmentLines = append(attachmentLines, attachmentLine(attachment.Kind, attachment.Path))
	}

	current, err := a.downloadMessageAttachments(ctx, b, msg)
	if err != nil {
		return nil, "", err
	}
	for _, attachment := range current {
		input = append(input, codexInputForAttachment(attachment.Path, attachment.Kind))
		attachmentLines = append(attachmentLines, attachmentLine(attachment.Kind, attachment.Path))
	}

	if displayText == "" && len(attachmentLines) > 0 {
		displayText = "Please inspect the attached file(s)."
	}
	if len(attachmentLines) > 0 {
		displayText = strings.TrimSpace(displayText + "\n\n" + strings.Join(attachmentLines, "\n"))
	}
	if displayText == "" {
		displayText = " "
	}
	return append(textInput(displayText), input...), displayText, nil
}

func (a *app) stageMessageAttachments(ctx context.Context, b *bot.Bot, msg *models.Message) (int, error) {
	attachments, err := a.downloadMessageAttachments(ctx, b, msg)
	if err != nil {
		return 0, err
	}
	for _, attachment := range attachments {
		if err := a.store.AddStagedAttachment(state.StagedAttachment{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			MessageID:       msg.ID,
			Path:            attachment.Path,
			Kind:            attachment.Kind,
			Name:            attachment.Name,
		}); err != nil {
			return 0, err
		}
	}
	return len(attachments), nil
}

type downloadedAttachment struct {
	Path string
	Kind string
	Name string
}

func (a *app) downloadMessageAttachments(ctx context.Context, b *bot.Bot, msg *models.Message) ([]downloadedAttachment, error) {
	var out []downloadedAttachment
	if len(msg.Photo) > 0 {
		photo := largestPhoto(msg.Photo)
		path, err := a.downloadTelegramFile(ctx, b, photo.FileID, fmt.Sprintf("photo-%d.jpg", msg.ID), msg)
		if err != nil {
			return nil, err
		}
		out = append(out, downloadedAttachment{Path: path, Kind: "image", Name: filepath.Base(path)})
	}

	if msg.Document != nil {
		name := safeFilename(msg.Document.FileName)
		if name == "" {
			name = fmt.Sprintf("document-%d", msg.ID)
		}
		path, err := a.downloadTelegramFile(ctx, b, msg.Document.FileID, name, msg)
		if err != nil {
			return nil, err
		}
		if isImagePath(path) || strings.HasPrefix(strings.ToLower(msg.Document.MimeType), "image/") {
			out = append(out, downloadedAttachment{Path: path, Kind: "image", Name: filepath.Base(path)})
		} else {
			out = append(out, downloadedAttachment{Path: path, Kind: "file", Name: filepath.Base(path)})
		}
	}
	return out, nil
}

func codexInputForAttachment(path, kind string) map[string]any {
	if kind == "image" {
		return map[string]any{"type": "localImage", "path": path}
	}
	return map[string]any{"type": "mention", "name": filepath.Base(path), "path": path}
}

func attachmentLine(kind, path string) string {
	if kind == "image" {
		return "Attached image: " + path
	}
	return "Attached file: " + path
}

func (a *app) downloadTelegramFile(ctx context.Context, b *bot.Bot, fileID, name string, msg *models.Message) (string, error) {
	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return "", err
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "Dexgram", "media", fmt.Sprintf("%d", msg.Chat.ID), fmt.Sprintf("%d", msg.MessageThreadID), fmt.Sprintf("%d", msg.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if ext := filepath.Ext(file.FilePath); filepath.Ext(name) == "" && ext != "" {
		name += ext
	}
	path := filepath.Join(dir, safeFilename(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.FileDownloadLink(file), nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("telegram file download returned HTTP %d", resp.StatusCode)
	}
	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = out.Close()
	}()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return "", err
	}
	return path, nil
}

var finalAnswerMarkdownLinkRE = regexp.MustCompile(`!?\[[^\]\n]*\]\(([^)\n]+)\)`)

func (a *app) sendFinalAnswerFiles(ctx context.Context, turn *telegramTurn, cwd, answer string) {
	if turn.SentFiles == nil {
		turn.SentFiles = map[string]bool{}
	}
	for _, path := range finalAnswerFilePaths(cwd, answer) {
		if turn.SentFiles[path] {
			continue
		}
		turn.SentFiles[path] = true
		if err := a.sendOutputFile(ctx, turn, path); err != nil {
			log.Printf("send final answer file %s: %v", path, err)
		}
	}
}

func (a *app) sendOutputFile(ctx context.Context, turn *telegramTurn, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	file := &models.InputFileUpload{Filename: filepath.Base(path), Data: f}
	if isImagePath(path) {
		_, err = a.bot.SendPhoto(ctx, &bot.SendPhotoParams{
			ChatID:              turn.ChatID,
			MessageThreadID:     turn.MessageThreadID,
			Photo:               file,
			Caption:             filepath.Base(path),
			DisableNotification: true,
		})
		return err
	}
	_, err = a.bot.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID:              turn.ChatID,
		MessageThreadID:     turn.MessageThreadID,
		Document:            file,
		Caption:             filepath.Base(path),
		DisableNotification: true,
	})
	return err
}

func finalAnswerFilePaths(cwd, answer string) []string {
	var paths []string
	seen := map[string]bool{}
	for _, match := range finalAnswerMarkdownLinkRE.FindAllStringSubmatch(answer, -1) {
		if len(match) < 2 {
			continue
		}
		path, ok := finalAnswerFilePath(cwd, match[1])
		if !ok || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

func resolveOutputPath(cwd, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cwd, path)
}

func finalAnswerFilePath(cwd, target string) (string, bool) {
	target = strings.TrimSpace(target)
	target = strings.Trim(target, "<>")
	target = strings.Trim(target, `"'`)
	if target == "" || isRemoteLinkTarget(target) {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(target), "file:") {
		parsed, err := url.Parse(target)
		if err != nil || parsed.Scheme != "file" {
			return "", false
		}
		target = parsed.Path
		if parsed.Host != "" {
			target = `\\` + parsed.Host + filepath.FromSlash(target)
		}
	}
	target = normalizeLinkedPath(target)
	for _, candidate := range []string{target, stripLineReference(target)} {
		path := resolveOutputPath(cwd, candidate)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return filepath.Clean(path), true
		}
	}
	return "", false
}

func isRemoteLinkTarget(target string) bool {
	lower := strings.ToLower(target)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "app://") ||
		strings.HasPrefix(lower, "plugin://") ||
		strings.HasPrefix(lower, "mailto:")
}

func normalizeLinkedPath(path string) string {
	path = strings.TrimSpace(path)
	if len(path) >= 3 && (path[0] == '/' || path[0] == '\\') &&
		isASCIILetter(path[1]) && (path[2] == '/' || path[2] == '\\') {
		path = string([]byte{path[1], ':'}) + path[2:]
	}
	if len(path) >= 3 && path[0] == '/' && isASCIILetter(path[1]) && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

func stripLineReference(path string) string {
	i := len(path) - 1
	for i >= 0 && path[i] >= '0' && path[i] <= '9' {
		i--
	}
	if i >= 0 && i < len(path)-1 && path[i] == ':' {
		return path[:i]
	}
	return path
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func largestPhoto(photos []models.PhotoSize) models.PhotoSize {
	best := photos[0]
	for _, photo := range photos[1:] {
		if photo.Width*photo.Height > best.Width*best.Height {
			best = photo
		}
	}
	return best
}

func isImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

func safeFilename(name string) string {
	name = strings.TrimSpace(name)
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	name = replacer.Replace(name)
	name = strings.Trim(name, ". ")
	if name == "" {
		return "file"
	}
	return name
}
