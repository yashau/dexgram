package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"dexgram/internal/codex"
	"dexgram/internal/codexprojects"
	"dexgram/internal/config"
	"dexgram/internal/state"

	"github.com/go-telegram/bot"
)

type app struct {
	cfg                   *config.Config
	bot                   *bot.Bot
	store                 *state.Store
	mu                    sync.Mutex
	projectsMu            sync.RWMutex
	active                map[string]*activeTurn
	actions               map[string]turnAction
	approvals             map[string]*pendingApproval
	inputs                map[string]*pendingInput
	approvalSeq           atomic.Int64
	actionSeq             atomic.Int64
	queueSeq              atomic.Int64
	inputSeq              atomic.Int64
	projects              []codexprojects.Project
	lastTypingAt          time.Time
	typingSuppressedUntil map[string]time.Time
}

type activeTurn struct {
	client         *codex.Client
	threadID       string
	ctx            context.Context
	cancel         context.CancelFunc
	conv           state.Conversation
	turns          map[string]*telegramTurn
	order          []string
	titleSyncItems map[string]bool
	pendingEvents  map[string][]codex.Event
	typing         bool
}

type telegramTurn struct {
	TurnID            string
	Queued            bool
	ChatID            int64
	MessageThreadID   int
	StatusMessageID   int
	Text              string
	Input             []map[string]any
	CollaborationMode string
	LastAgent         string
	FinalAnswer       string
	Buffers           map[string]string
	Initial           *liveTextMessage
	RunLog            *runLog
	SentFiles         map[string]bool
	CompactionDraft   *liveTextMessage
	CompactionItemID  string
	CompactionCancel  context.CancelFunc
}

type turnAction struct {
	Key    string
	TurnID string
}

type pendingApproval struct {
	ch chan approvalDecision
}

type pendingInput struct {
	ch              chan inputDecision
	chatID          int64
	messageThreadID int
	promptMessageID int
	questions       []inputQuestion
}

type approvalDecision struct {
	result any
	err    error
}

type inputDecision struct {
	result map[string]any
	err    error
}

type approvalRequestParams struct {
	Reason                      string   `json:"reason"`
	Command                     string   `json:"command"`
	ProposedExecpolicyAmendment []string `json:"proposedExecpolicyAmendment"`
}
