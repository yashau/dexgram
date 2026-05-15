package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"dexgram/internal/codexstate"
	"dexgram/internal/state"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (a *app) handleSideCommand(ctx context.Context, b *bot.Bot, msg *models.Message, prompt string) {
	parent, ok, err := a.store.Get(msg.Chat.ID, msg.MessageThreadID)
	if err != nil {
		log.Printf("read conversation for side command: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Could not read Dexgram state: " + err.Error(),
		})
		return
	}
	if !ok || parent.CodexThreadID == "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Start this Codex chat first, then use /side or /btw to fork its current context.",
		})
		return
	}
	if parent.ProjectName == "" {
		parent.Projectless = true
	}

	index, err := a.store.NextSideIndex(parent.ChatID, parent.MessageThreadID)
	if err != nil {
		log.Printf("read next side-chat index: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Could not prepare a side chat: " + err.Error(),
		})
		return
	}
	sideName := sideTopicTitle(sideTopicBaseTitle(parent), index)
	threadID, cwd, err := a.forkTopicThread(ctx, msg.Chat.ID, msg.MessageThreadID, parent)
	if err != nil {
		log.Printf("fork Codex thread: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Could not fork the Codex thread:\n\n" + err.Error(),
		})
		return
	}

	topic, err := b.CreateForumTopic(ctx, &bot.CreateForumTopicParams{
		ChatID: msg.Chat.ID,
		Name:   sideName,
	})
	if err != nil {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: msg.MessageThreadID,
			Text:            "Codex forked the thread, but Dexgram could not create a Telegram topic: " + err.Error(),
		})
		return
	}
	if topic.Name != "" {
		sideName = topic.Name
	}

	side := state.Conversation{
		ChatID:                msg.Chat.ID,
		MessageThreadID:       topic.MessageThreadID,
		CodexThreadID:         threadID,
		ProjectName:           parent.ProjectName,
		CWD:                   cwd,
		Projectless:           parent.Projectless,
		TopicTitle:            sideName,
		TopicNamed:            true,
		SideChat:              true,
		ParentChatID:          parent.ChatID,
		ParentMessageThreadID: parent.MessageThreadID,
		ParentCodexThreadID:   parent.CodexThreadID,
		SideIndex:             index,
	}
	if side.CWD == "" {
		side.CWD = parent.CWD
	}
	if err := a.store.Upsert(side); err != nil {
		log.Printf("store side topic: %v", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: topic.MessageThreadID,
			Text:            "Side topic created, but Dexgram could not save its mapping: " + err.Error(),
		})
		return
	}
	if side.Projectless {
		if err := codexstate.RegisterProjectlessThread(threadID, projectlessRoot()); err != nil {
			log.Printf("register projectless side Codex thread: %v", err)
		}
	}

	_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:              msg.Chat.ID,
		MessageThreadID:     msg.MessageThreadID,
		Text:                "Opened side chat: " + sideName,
		DisableNotification: true,
	})
	if strings.TrimSpace(prompt) == "" {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          msg.Chat.ID,
			MessageThreadID: topic.MessageThreadID,
			Text:            fmt.Sprintf("Side chat forked from %s. Send a message here to continue separately.", sideTopicBaseTitle(parent)),
		})
		return
	}

	sideMsg := *msg
	sideMsg.MessageThreadID = topic.MessageThreadID
	sideMsg.ID = 0
	sideMsg.Text = strings.TrimSpace(prompt)
	sideMsg.Caption = ""
	a.handlePrompt(ctx, b, &sideMsg, sideMsg.Text)
}

func sideTopicBaseTitle(conv state.Conversation) string {
	if strings.TrimSpace(conv.TopicTitle) != "" {
		return conv.TopicTitle
	}
	if strings.TrimSpace(conv.ProjectName) != "" {
		return conv.ProjectName
	}
	return "Side chat"
}
