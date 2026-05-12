package main

import (
	"log"
	"strings"

	"dexgram/internal/codexprojects"
)

func (a *app) refreshProjects() ([]codexprojects.Project, error) {
	projects, err := codexprojects.Load()
	if err != nil {
		return nil, err
	}
	a.projectsMu.Lock()
	a.projects = projects
	a.projectsMu.Unlock()
	log.Printf("loaded %d Codex projects", len(projects))
	return projects, nil
}

func (a *app) projectByIndex(index int) (codexprojects.Project, bool) {
	a.projectsMu.RLock()
	defer a.projectsMu.RUnlock()
	if index < 0 || index >= len(a.projects) {
		return codexprojects.Project{}, false
	}
	return a.projects[index], true
}

func projectIndex(projects []codexprojects.Project, project codexprojects.Project) int {
	for i, candidate := range projects {
		if strings.EqualFold(candidate.Path, project.Path) {
			return i
		}
	}
	return -1
}
