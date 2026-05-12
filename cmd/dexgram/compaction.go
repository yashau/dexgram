package main

import (
	"context"
	"strings"
	"time"

	"dexgram/internal/codex"

	"github.com/go-telegram/bot"
)

const compactionDraftInterval = 650 * time.Millisecond

func (t *telegramTurn) startCompactionDraft(ctx context.Context, b *bot.Bot, itemID string) {
	if itemID != "" {
		t.CompactionItemID = itemID
	}
	if t.CompactionCancel != nil {
		return
	}
	if t.CompactionDraft == nil {
		t.CompactionDraft = &liveTextMessage{
			ctx:             ctx,
			bot:             b,
			chatID:          t.ChatID,
			messageThreadID: t.MessageThreadID,
		}
	}
	draftCtx, cancel := context.WithCancel(ctx)
	t.CompactionCancel = cancel
	go animateCompactionDraft(draftCtx, t.CompactionDraft)
}

func (t *telegramTurn) stopCompactionDraft() {
	if t.CompactionCancel == nil {
		return
	}
	t.CompactionCancel()
	t.CompactionCancel = nil
	t.CompactionItemID = ""
}

func (t *telegramTurn) isCompactionItemID(itemID string) bool {
	return itemID != "" && t.CompactionItemID == itemID
}

func animateCompactionDraft(ctx context.Context, msg *liveTextMessage) {
	ticker := time.NewTicker(compactionDraftInterval)
	defer ticker.Stop()

	for frame := 0; ; frame++ {
		msg.draft(compactionDraftFrame(frame))
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func compactionDraftFrame(frame int) string {
	return "Compacting context" + strings.Repeat(".", frame%3+1)
}

func isCompactionNoticeItem(item codex.ThreadItem) bool {
	return item.Type == "compaction"
}
