package main

import (
	"testing"

	"dexgram/internal/codexprojects"
)

func TestProjectIndexMatchesPathCaseInsensitively(t *testing.T) {
	projects := []codexprojects.Project{
		{Name: "Dexgram", Path: `C:\Users\Yashau\Projects\dexgram`},
		{Name: "Other", Path: `C:\Users\Yashau\Projects\other`},
	}

	if got := projectIndex(projects, codexprojects.Project{Path: `c:\users\yashau\projects\DEXGRAM`}); got != 0 {
		t.Fatalf("project index = %d, want 0", got)
	}
	if got := projectIndex(projects, codexprojects.Project{Path: `C:\missing`}); got != -1 {
		t.Fatalf("missing project index = %d, want -1", got)
	}
}

func TestProjectByIndexBoundsChecks(t *testing.T) {
	app := newTestApp()
	app.projects = []codexprojects.Project{{Name: "Dexgram", Path: `C:\dexgram`}}

	project, ok := app.projectByIndex(0)
	if !ok || project.Name != "Dexgram" {
		t.Fatalf("project by index = %#v ok=%v", project, ok)
	}
	if _, ok := app.projectByIndex(-1); ok {
		t.Fatal("negative index should not resolve")
	}
	if _, ok := app.projectByIndex(1); ok {
		t.Fatal("out of range index should not resolve")
	}
}
