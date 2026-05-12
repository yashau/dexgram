package codexprojects

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Project struct {
	Name string
	Path string
}

func Load() ([]Project, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	statePath := filepath.Join(home, ".codex", ".codex-global-state.json")
	b, err := os.ReadFile(statePath)
	if err != nil {
		return nil, err
	}
	var raw struct {
		ProjectOrder                []string `json:"project-order"`
		ElectronSavedWorkspaceRoots []string `json:"electron-saved-workspace-roots"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var projects []Project
	for _, p := range append(raw.ProjectOrder, raw.ElectronSavedWorkspaceRoots...) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		key := strings.ToLower(abs)
		if seen[key] {
			continue
		}
		seen[key] = true
		projects = append(projects, Project{
			Name: projectName(abs),
			Path: abs,
		})
	}
	return projects, nil
}

func Match(projects []Project, query string, limit int) []Project {
	query = normalize(query)
	if query == "" {
		return nil
	}
	type scored struct {
		project Project
		score   int
	}
	var matches []scored
	for _, project := range projects {
		name := normalize(project.Name)
		path := normalize(project.Path)
		score := score(name, path, query)
		if score > 0 {
			matches = append(matches, scored{project: project, score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return strings.ToLower(matches[i].project.Name) < strings.ToLower(matches[j].project.Name)
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	out := make([]Project, 0, len(matches))
	for _, match := range matches {
		out = append(out, match.project)
	}
	return out
}

func projectName(path string) string {
	clean := filepath.Clean(path)
	name := filepath.Base(clean)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return clean
	}
	return name
}

func score(name, path, query string) int {
	switch {
	case name == query:
		return 100
	case strings.HasPrefix(name, query):
		return 80
	case strings.Contains(name, query):
		return 60
	case strings.Contains(path, query):
		return 40
	case acronym(name) == query:
		return 35
	case subsequence(name, query):
		return 20
	default:
		return 0
	}
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer("-", "", "_", "", " ", "", ".", "", "/", "", "\\", "")
	return replacer.Replace(s)
}

func acronym(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '_' || r == ' ' || r == '.' || r == '/' || r == '\\'
	})
	var b strings.Builder
	for _, field := range fields {
		if field != "" {
			b.WriteByte(strings.ToLower(field)[0])
		}
	}
	return b.String()
}

func subsequence(s, query string) bool {
	if query == "" {
		return true
	}
	i := 0
	for _, r := range s {
		if byte(r) == query[i] {
			i++
			if i == len(query) {
				return true
			}
		}
	}
	return false
}
