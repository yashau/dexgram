package codexstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const globalStateFile = ".codex-global-state.json"

func RegisterProjectlessThread(threadID, workspaceRoot string) error {
	if threadID == "" || workspaceRoot == "" {
		return nil
	}
	statePath, err := globalStatePath()
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			raw = []byte("{}")
		} else {
			return fmt.Errorf("read Codex global state: %w", err)
		}
	}
	var state map[string]any
	if len(raw) == 0 {
		state = map[string]any{}
	} else if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("parse Codex global state: %w", err)
	}
	if state == nil {
		state = map[string]any{}
	}

	state["projectless-thread-ids"] = appendStringUnique(stringSlice(state["projectless-thread-ids"]), threadID)
	hints := stringMap(state["thread-workspace-root-hints"])
	hints[threadID] = workspaceRoot
	state["thread-workspace-root-hints"] = hints

	out, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode Codex global state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return fmt.Errorf("create Codex state directory: %w", err)
	}
	return os.WriteFile(statePath, out, 0o600)
}

func globalStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", globalStateFile), nil
}

func stringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if ok {
			out = append(out, text)
		}
	}
	return out
}

func appendStringUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func stringMap(v any) map[string]string {
	out := map[string]string{}
	raw, ok := v.(map[string]any)
	if !ok {
		return out
	}
	for key, value := range raw {
		text, ok := value.(string)
		if ok {
			out[key] = text
		}
	}
	return out
}
