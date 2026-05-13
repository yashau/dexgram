package codexstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRegisterProjectlessThreadCreatesAndUpdatesGlobalState(t *testing.T) {
	home := setCodexStateTestHome(t)
	workspace := filepath.Join(home, "Documents", "Codex", "2026-05-13", "chat")

	if err := RegisterProjectlessThread("thread-1", workspace); err != nil {
		t.Fatal(err)
	}
	if err := RegisterProjectlessThread("thread-1", workspace); err != nil {
		t.Fatal(err)
	}

	state := readTestGlobalState(t, home)
	ids := stringSlice(state["projectless-thread-ids"])
	if !reflect.DeepEqual(ids, []string{"thread-1"}) {
		t.Fatalf("projectless ids mismatch: %#v", ids)
	}
	hints := stringMap(state["thread-workspace-root-hints"])
	if hints["thread-1"] != workspace {
		t.Fatalf("workspace hint mismatch: %#v", hints)
	}
}

func TestRegisterProjectlessThreadPreservesExistingState(t *testing.T) {
	home := setCodexStateTestHome(t)
	path := filepath.Join(home, ".codex", globalStateFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	initial := `{
		"theme": "dark",
		"projectless-thread-ids": ["existing"],
		"thread-workspace-root-hints": {"existing": "C:\\old"}
	}`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RegisterProjectlessThread("thread-2", `C:\new`); err != nil {
		t.Fatal(err)
	}

	state := readTestGlobalState(t, home)
	if state["theme"] != "dark" {
		t.Fatalf("expected unrelated state to be preserved, got %#v", state)
	}
	if ids := stringSlice(state["projectless-thread-ids"]); !reflect.DeepEqual(ids, []string{"existing", "thread-2"}) {
		t.Fatalf("projectless ids mismatch: %#v", ids)
	}
	hints := stringMap(state["thread-workspace-root-hints"])
	if hints["existing"] != `C:\old` || hints["thread-2"] != `C:\new` {
		t.Fatalf("workspace hints mismatch: %#v", hints)
	}
}

func TestRegisterProjectlessThreadIgnoresMissingInputs(t *testing.T) {
	home := setCodexStateTestHome(t)

	if err := RegisterProjectlessThread("", `C:\workspace`); err != nil {
		t.Fatal(err)
	}
	if err := RegisterProjectlessThread("thread", ""); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(home, ".codex", globalStateFile)); !os.IsNotExist(err) {
		t.Fatalf("expected no state file to be written, stat err=%v", err)
	}
}

func TestRegisterProjectlessThreadRejectsInvalidExistingState(t *testing.T) {
	home := setCodexStateTestHome(t)
	path := filepath.Join(home, ".codex", globalStateFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RegisterProjectlessThread("thread", `C:\workspace`); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestStringHelpersFilterUnexpectedTypes(t *testing.T) {
	if got := stringSlice([]any{"a", 1, "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("stringSlice mismatch: %#v", got)
	}
	rawMap := map[string]any{"a": "one", "b": 2}
	if got := stringMap(rawMap); !reflect.DeepEqual(got, map[string]string{"a": "one"}) {
		t.Fatalf("stringMap mismatch: %#v", got)
	}
}

func setCodexStateTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func readTestGlobalState(t *testing.T, home string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, ".codex", globalStateFile))
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatal(err)
	}
	return state
}
