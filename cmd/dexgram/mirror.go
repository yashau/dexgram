package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dexgram/internal/codex"
	"dexgram/internal/state"

	"github.com/fsnotify/fsnotify"
)

const (
	desktopMirrorReconcileInterval = 15 * time.Second
	desktopMirrorFallbackInterval  = 30 * time.Second
)

type desktopMirrorHandle struct {
	threadID string
	cancel   context.CancelFunc
	done     <-chan struct{}
}

type sessionFileState struct {
	modTime time.Time
	size    int64
}

func (a *app) runDesktopMirrors(ctx context.Context) {
	handles := map[string]desktopMirrorHandle{}
	defer func() {
		for _, handle := range handles {
			handle.cancel()
		}
	}()

	reconcile := func() {
		for key, handle := range handles {
			select {
			case <-handle.done:
				delete(handles, key)
			default:
			}
		}

		convs, err := a.store.ListConversations()
		if err != nil {
			log.Printf("list conversations for desktop mirror: %v", err)
			return
		}
		desired := map[string]state.Conversation{}
		for _, conv := range convs {
			if conv.CodexThreadID == "" || !a.allowedChat(conv.ChatID) {
				continue
			}
			key := conversationKey(conv.ChatID, conv.MessageThreadID)
			desired[key] = conv
			if handle, ok := handles[key]; ok && handle.threadID == conv.CodexThreadID {
				continue
			}
			if handle, ok := handles[key]; ok {
				handle.cancel()
			}
			mirrorCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			handles[key] = desktopMirrorHandle{threadID: conv.CodexThreadID, cancel: cancel, done: done}
			go func(key string, conv state.Conversation) {
				defer close(done)
				a.mirrorConversation(mirrorCtx, key, conv)
			}(key, conv)
		}
		for key, handle := range handles {
			if _, ok := desired[key]; !ok {
				handle.cancel()
				delete(handles, key)
			}
		}
	}

	reconcile()
	ticker := time.NewTicker(desktopMirrorReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reconcile()
		case <-a.mirrorRefresh:
			reconcile()
		}
	}
}

func (a *app) mirrorConversation(ctx context.Context, key string, conv state.Conversation) {
	c, err := codex.StartStdioWithOptions(ctx, codex.StartOptions{
		CLIPath:    a.cfg.Codex.CLIPath,
		WorkingDir: appServerWorkingDir(conv),
	})
	if err != nil {
		log.Printf("start desktop mirror app-server key=%s thread_id=%s: %v", key, conv.CodexThreadID, err)
		return
	}
	defer func() {
		_ = c.Close()
	}()
	go func() {
		for err := range c.Errors() {
			log.Printf("codex app-server desktop mirror key=%s: %v", key, err)
		}
	}()
	if err := a.resumeCodexThread(ctx, c, conv.CodexThreadID); err != nil {
		log.Printf("resume desktop mirror thread key=%s thread_id=%s: %v", key, conv.CodexThreadID, err)
		return
	}

	sessionPath, hasSessionPath := codexSessionFilePath(conv.CodexThreadID)
	if hasSessionPath {
		log.Printf("desktop mirror watching session file key=%s thread_id=%s path=%s", key, conv.CodexThreadID, sessionPath)
	} else {
		log.Printf("desktop mirror session file not found key=%s thread_id=%s; falling back to periodic thread reads", key, conv.CodexThreadID)
	}
	lastFallback := time.Now().Add(-desktopMirrorFallbackInterval)
	lastPathLookup := time.Now()
	sessionState, _ := statSessionFile(sessionPath)
	watcher, err := newSessionFileWatcher(sessionPath)
	if err != nil {
		log.Printf("desktop mirror file watcher key=%s thread_id=%s: %v", key, conv.CodexThreadID, err)
	}
	defer func() {
		closeSessionFileWatcher(watcher)
	}()
	fallback := time.NewTicker(desktopMirrorFallbackInterval)
	defer fallback.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcherEvents(watcher):
			if !ok {
				watcher = nil
				continue
			}
			if !sessionFileEventMatches(event, sessionPath) {
				continue
			}
			nextState, changed := sessionFileChanged(sessionPath, sessionState)
			if !changed {
				continue
			}
			sessionState = nextState
			lastFallback = time.Now()
			var stop bool
			conv, stop = a.mirrorConversationOnce(ctx, c, key, conv)
			if stop {
				return
			}
		case err, ok := <-watcherErrors(watcher):
			if !ok {
				watcher = nil
				continue
			}
			log.Printf("desktop mirror file watcher key=%s thread_id=%s: %v", key, conv.CodexThreadID, err)
		case <-fallback.C:
			if !hasSessionPath && time.Since(lastPathLookup) >= desktopMirrorFallbackInterval {
				lastPathLookup = time.Now()
				if sessionPath, hasSessionPath = codexSessionFilePath(conv.CodexThreadID); hasSessionPath {
					closeSessionFileWatcher(watcher)
					sessionState, _ = statSessionFile(sessionPath)
					watcher, err = newSessionFileWatcher(sessionPath)
					if err != nil {
						log.Printf("desktop mirror file watcher key=%s thread_id=%s: %v", key, conv.CodexThreadID, err)
					}
					log.Printf("desktop mirror found session file key=%s thread_id=%s path=%s", key, conv.CodexThreadID, sessionPath)
				}
			}
			if time.Since(lastFallback) >= desktopMirrorFallbackInterval {
				lastFallback = time.Now()
				var stop bool
				conv, stop = a.mirrorConversationOnce(ctx, c, key, conv)
				if stop {
					return
				}
			}
		}
	}
}

func (a *app) mirrorConversationOnce(ctx context.Context, c *codex.Client, key string, conv state.Conversation) (state.Conversation, bool) {
	updated, err := a.mirrorCompletedDesktopTurns(ctx, c, key, conv)
	if err != nil {
		if errors.Is(err, errTelegramTopicGone) {
			log.Printf("desktop mirror stopped for deleted telegram topic key=%s thread_id=%s", key, conv.CodexThreadID)
			return conv, true
		}
		log.Printf("desktop mirror sync key=%s thread_id=%s: %v", key, conv.CodexThreadID, err)
		return conv, false
	}
	return updated, false
}

func (a *app) mirrorCompletedDesktopTurns(ctx context.Context, c *codex.Client, key string, conv state.Conversation) (state.Conversation, error) {
	if a.activeSession(key) != nil {
		return conv, nil
	}
	resume, err := a.resumeCodexThreadResult(ctx, c, conv.CodexThreadID)
	if err != nil {
		return conv, err
	}
	thread := resume.Thread
	if len(thread.Turns) == 0 {
		var read codex.ThreadReadResponse
		if err := c.Call(ctx, "thread/read", map[string]any{"threadId": conv.CodexThreadID}, &read); err != nil {
			return conv, err
		}
		thread = read.Thread
	}
	if len(thread.Turns) == 0 {
		return conv, nil
	}
	if conv.LastSyncedTurnID == "" {
		latest := latestCompletedTurnID(thread.Turns)
		if latest == "" {
			return conv, nil
		}
		conv.LastSyncedTurnID = latest
		if err := a.store.Upsert(conv); err != nil {
			return conv, err
		}
		log.Printf("desktop mirror initialized sync marker key=%s thread_id=%s turn_id=%s", key, conv.CodexThreadID, latest)
		return conv, nil
	}

	turns, found := completedTurnsAfterMarker(thread.Turns, conv.LastSyncedTurnID)
	if !found {
		latest := latestCompletedTurnID(thread.Turns)
		if latest != "" && latest != conv.LastSyncedTurnID {
			log.Printf("desktop mirror marker not found key=%s thread_id=%s marker=%s latest=%s", key, conv.CodexThreadID, conv.LastSyncedTurnID, latest)
			conv.LastSyncedTurnID = latest
			if err := a.store.Upsert(conv); err != nil {
				return conv, err
			}
		}
		return conv, nil
	}
	for _, turn := range turns {
		if a.shouldSkipTelegramOriginTurn(conv.CodexThreadID, turn) {
			conv.LastSyncedTurnID = turn.ID
			continue
		}
		if err := renderHistoricalTurnSilent(ctx, a.bot, conv.ChatID, conv.MessageThreadID, turn); err != nil {
			if isTelegramTopicGoneError(err) {
				if deleteErr := a.forgetDeletedTelegramTopic(conv); deleteErr != nil {
					return conv, deleteErr
				}
				return conv, errTelegramTopicGone
			}
			return conv, err
		}
		conv.LastSyncedTurnID = turn.ID
	}
	if len(turns) > 0 {
		if err := a.store.Upsert(conv); err != nil {
			return conv, err
		}
	}
	return conv, nil
}

func (a *app) requestMirrorRefresh() {
	if a.mirrorRefresh == nil {
		return
	}
	select {
	case a.mirrorRefresh <- struct{}{}:
	default:
	}
}

func completedTurnsAfterMarker(turns []codex.Turn, marker string) ([]codex.Turn, bool) {
	var out []codex.Turn
	seen := marker == ""
	for _, turn := range turns {
		if seen && turn.Status == "completed" {
			out = append(out, turn)
		}
		if turn.ID == marker {
			seen = true
		}
	}
	return out, seen
}

func latestCompletedTurnID(turns []codex.Turn) string {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Status == "completed" {
			return turns[i].ID
		}
	}
	return ""
}

func conversationKey(chatID int64, messageThreadID int) string {
	return fmt.Sprintf("%d:%d", chatID, messageThreadID)
}

func newSessionFileWatcher(path string) (*fsnotify.Watcher, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(filepath.Dir(path)); err != nil {
		_ = watcher.Close()
		return nil, err
	}
	return watcher, nil
}

func closeSessionFileWatcher(watcher *fsnotify.Watcher) {
	if watcher != nil {
		_ = watcher.Close()
	}
}

func watcherEvents(watcher *fsnotify.Watcher) <-chan fsnotify.Event {
	if watcher == nil {
		return nil
	}
	return watcher.Events
}

func watcherErrors(watcher *fsnotify.Watcher) <-chan error {
	if watcher == nil {
		return nil
	}
	return watcher.Errors
}

func sessionFileEventMatches(event fsnotify.Event, path string) bool {
	if strings.TrimSpace(path) == "" || event.Name == "" {
		return false
	}
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) && !event.Has(fsnotify.Remove) {
		return false
	}
	cleanEvent := filepath.Clean(event.Name)
	cleanPath := filepath.Clean(path)
	return cleanEvent == cleanPath || cleanEvent == filepath.Dir(cleanPath) || filepath.Dir(cleanEvent) == filepath.Dir(cleanPath)
}

func statSessionFile(path string) (sessionFileState, bool) {
	if strings.TrimSpace(path) == "" {
		return sessionFileState{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return sessionFileState{}, false
	}
	return sessionFileState{modTime: info.ModTime(), size: info.Size()}, true
}

func sessionFileChanged(path string, previous sessionFileState) (sessionFileState, bool) {
	next, ok := statSessionFile(path)
	if !ok {
		return previous, false
	}
	if previous.size == 0 && previous.modTime.IsZero() {
		return next, true
	}
	return next, next.size != previous.size || !next.modTime.Equal(previous.modTime)
}

func codexSessionFilePath(threadID string) (string, bool) {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	root := filepath.Join(home, ".codex", "sessions")
	var newest string
	var newestMod time.Time
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".jsonl") || !strings.Contains(name, threadID) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if newest == "" || info.ModTime().After(newestMod) {
			newest = path
			newestMod = info.ModTime()
		}
		return nil
	}); err != nil {
		return "", false
	}
	return newest, newest != ""
}
