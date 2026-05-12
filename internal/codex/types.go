package codex

import "encoding/json"

type ThreadListResponse struct {
	Data []Thread `json:"data"`
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

type TurnStartResponse struct {
	Turn Turn `json:"turn"`
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
