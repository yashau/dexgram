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
	configPath            string
	configState           configFileState
	logPath               string
	mu                    sync.Mutex
	transcriptMu          sync.Mutex
	projectsMu            sync.RWMutex
	active                map[string]*activeTurn
	actions               map[string]turnAction
	freshTopics           map[string]*pendingFreshTopic
	sessionBrowsers       map[string]*sessionBrowser
	approvals             map[string]*pendingApproval
	inputs                map[string]*pendingInput
	approvalSeq           atomic.Int64
	actionSeq             atomic.Int64
	freshSeq              atomic.Int64
	sessionBrowserSeq     atomic.Int64
	queueSeq              atomic.Int64
	inputSeq              atomic.Int64
	mirrorRefresh         chan struct{}
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
	SourceMessageID   int
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

type pendingFreshTopic struct {
	chatID            int64
	messageThreadID   int
	replyMessageID    int
	input             []map[string]any
	displayText       string
	collaborationMode string
	createdAt         time.Time
}

type sessionBrowser struct {
	chatID          int64
	messageThreadID int
	pendingFreshKey string
	query           string
	threads         []codex.Thread
	projects        []sessionProjectGroup
	projectIndex    int
	page            int
	createdAt       time.Time
}

type sessionProjectGroup struct {
	Name        string
	CWD         string
	ThreadCount int
	ThreadIDs   []int
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
	Reason         string `json:"reason"`
	Command        string `json:"command"`
	CommandActions []struct {
		Command string `json:"command"`
	} `json:"commandActions"`
	GrantRoot                   string   `json:"grantRoot"`
	ProposedExecpolicyAmendment []string `json:"proposedExecpolicyAmendment"`
}
