package codexprojects

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadReadsGlobalStateDedupesAndSkipsEmptyProjects(t *testing.T) {
	home := setTestHome(t)
	first := filepath.Join(home, "Projects", "Dexgram")
	duplicate := filepath.Join(home, "projects", "dexgram")
	second := filepath.Join(home, "Projects", "Other")
	writeGlobalState(t, home, map[string]any{
		"project-order":                  []string{first, "  ", duplicate},
		"electron-saved-workspace-roots": []string{second, first},
	})

	projects, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	want := []Project{
		{Name: "Dexgram", Path: filepath.Clean(first)},
		{Name: "Other", Path: filepath.Clean(second)},
	}
	if !reflect.DeepEqual(projects, want) {
		t.Fatalf("projects mismatch\ngot:  %#v\nwant: %#v", projects, want)
	}
}

func TestLoadRejectsInvalidJSON(t *testing.T) {
	home := setTestHome(t)
	writeRawGlobalState(t, home, `{`)

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestMatchScoresSortsAndLimitsProjects(t *testing.T) {
	projects := []Project{
		{Name: "Codex Gram", Path: filepath.Join("C:", "work", "codexgram")},
		{Name: "Dexgram", Path: filepath.Join("C:", "work", "dexgram")},
		{Name: "Demo Explorer", Path: filepath.Join("C:", "work", "demo-explorer")},
		{Name: "Alpha", Path: filepath.Join("C:", "work", "contains-dex")},
	}

	got := Match(projects, "dex", 3)
	want := []Project{projects[1], projects[0], projects[3]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("matches mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestMatchNormalizesQueryAndReturnsNoEmptyQueryMatches(t *testing.T) {
	projects := []Project{{Name: "Codex Gram", Path: filepath.Join("C:", "work", "codexgram")}}

	if got := Match(projects, " codex-gram ", 10); !reflect.DeepEqual(got, projects) {
		t.Fatalf("expected normalized query match, got %#v", got)
	}
	if got := Match(projects, "   ", 10); got != nil {
		t.Fatalf("expected nil for empty query, got %#v", got)
	}
}

func TestProjectNameHandlesRootAndNormalPaths(t *testing.T) {
	if got := projectName(filepath.Join("C:", "work", "dexgram")); got != "dexgram" {
		t.Fatalf("projectName returned %q", got)
	}
	if got := projectName(""); got == "" {
		t.Fatal("expected a non-empty fallback name for empty path")
	}
}

func setTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func writeGlobalState(t *testing.T, home string, state map[string]any) {
	t.Helper()
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	writeRawGlobalState(t, home, string(b))
}

func writeRawGlobalState(t *testing.T, home, content string) {
	t.Helper()
	path := filepath.Join(home, ".codex", ".codex-global-state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
