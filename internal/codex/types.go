package codex

import (
	"bytes"
	"encoding/json"
)

type ThreadListResponse struct {
	Data          []Thread        `json:"data"`
	Items         []Thread        `json:"items"`
	Threads       []Thread        `json:"threads"`
	NextCursor    json.RawMessage `json:"nextCursor"`
	NextCursorAlt json.RawMessage `json:"next_cursor"`
}

type ThreadReadResponse struct {
	Thread Thread `json:"thread"`
}

type ThreadStartResponse struct {
	Thread Thread `json:"thread"`
	Model  string `json:"model"`
	Cwd    string `json:"cwd"`
}

type ThreadResumeResponse struct {
	Thread Thread `json:"thread"`
	Model  string `json:"model"`
	Cwd    string `json:"cwd"`
}

type ThreadForkResponse struct {
	Thread Thread `json:"thread"`
	Model  string `json:"model"`
	Cwd    string `json:"cwd"`
}

type ThreadGoalGetResponse struct {
	Goal *ThreadGoal `json:"goal"`
}

type ThreadGoal struct {
	ThreadID        string   `json:"threadId"`
	Objective       string   `json:"objective"`
	Status          string   `json:"status"`
	TokenBudget     *float64 `json:"tokenBudget"`
	TokensUsed      float64  `json:"tokensUsed"`
	TimeUsedSeconds float64  `json:"timeUsedSeconds"`
	CreatedAt       float64  `json:"createdAt"`
	UpdatedAt       float64  `json:"updatedAt"`
}

type TurnStartResponse struct {
	Turn Turn `json:"turn"`
}

type ModelListResponse struct {
	Data []ModelOption `json:"data"`
}

type AccountRateLimitsResponse struct {
	RateLimits          RateLimitSnapshot             `json:"rateLimits"`
	RateLimitsByLimitID map[string]*RateLimitSnapshot `json:"rateLimitsByLimitId"`
}

type RateLimitSnapshot struct {
	LimitID              string           `json:"limitId"`
	LimitName            string           `json:"limitName"`
	Primary              *RateLimitWindow `json:"primary"`
	Secondary            *RateLimitWindow `json:"secondary"`
	PlanType             string           `json:"planType"`
	RateLimitReachedType string           `json:"rateLimitReachedType"`
}

type RateLimitWindow struct {
	UsedPercent        float64  `json:"usedPercent"`
	WindowDurationMins *float64 `json:"windowDurationMins"`
	ResetsAt           *float64 `json:"resetsAt"`
}

type ModelOption struct {
	ID                       string                   `json:"id"`
	Model                    string                   `json:"model"`
	DisplayName              string                   `json:"displayName"`
	DefaultReasoningEffort   string                   `json:"defaultReasoningEffort"`
	SupportedReasoningEffort []ReasoningEffortWrapper `json:"supportedReasoningEfforts"`
	IsDefault                bool                     `json:"isDefault"`
	Hidden                   bool                     `json:"hidden"`
}

func (m ModelOption) Name() string {
	if m.Model != "" {
		return m.Model
	}
	return m.ID
}

type ReasoningEffortWrapper struct {
	ReasoningEffort    string `json:"reasoning_effort"`
	ReasoningEffortAlt string `json:"reasoningEffort"`
}

func (r *ReasoningEffortWrapper) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		r.ReasoningEffort = s
		return nil
	}
	type wrapper ReasoningEffortWrapper
	var out wrapper
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	*r = ReasoningEffortWrapper(out)
	return nil
}

type Thread struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Preview   string          `json:"preview"`
	Cwd       string          `json:"cwd"`
	UpdatedAt float64         `json:"updatedAt"`
	CreatedAt float64         `json:"createdAt"`
	Status    json.RawMessage `json:"status"`
	Turns     []Turn          `json:"turns"`
}

type Turn struct {
	ID     string          `json:"id"`
	Status string          `json:"status"`
	Items  []ThreadItem    `json:"items"`
	Error  json.RawMessage `json:"error"`
}

type ThreadItem struct {
	Type             string          `json:"type"`
	ID               string          `json:"id"`
	Text             string          `json:"text"`
	Phase            *string         `json:"phase"`
	Content          json.RawMessage `json:"content"`
	Command          string          `json:"command"`
	Cwd              string          `json:"cwd"`
	Path             string          `json:"path"`
	Status           string          `json:"status"`
	AggregatedOutput *string         `json:"aggregatedOutput"`
	ExitCode         *int            `json:"exitCode"`
	DurationMs       *int64          `json:"durationMs"`
	Server           string          `json:"server"`
	Tool             string          `json:"tool"`
	Query            string          `json:"query"`
	Result           string          `json:"result"`
	SavedPath        string          `json:"savedPath"`
	RevisedPrompt    *string         `json:"revisedPrompt"`
	Changes          []FileChange    `json:"changes"`
}

func (i *ThreadItem) UnmarshalJSON(data []byte) error {
	type threadItem ThreadItem
	var raw struct {
		*threadItem
		Result json.RawMessage `json:"result"`
	}
	raw.threadItem = (*threadItem)(i)
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	result, err := decodeStringLike(raw.Result)
	if err != nil {
		return err
	}
	i.Result = result
	return nil
}

func decodeStringLike(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", err
	}
	return compact.String(), nil
}

type FileChange struct {
	Path string          `json:"path"`
	Kind json.RawMessage `json:"kind"`
	Diff string          `json:"diff"`
}

type ItemCompletedNotification struct {
	ThreadID string     `json:"threadId"`
	TurnID   string     `json:"turnId"`
	Item     ThreadItem `json:"item"`
}

type ItemStartedNotification struct {
	ThreadID string     `json:"threadId"`
	TurnID   string     `json:"turnId"`
	Item     ThreadItem `json:"item"`
}

type AgentMessageDeltaNotification struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
}

type CommandOutputDeltaNotification struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
	Output   string `json:"output"`
}

type TurnCompletedNotification struct {
	ThreadID string `json:"threadId"`
	Turn     Turn   `json:"turn"`
}
