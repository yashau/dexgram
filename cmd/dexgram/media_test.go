package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFinalAnswerFilePathsOnlyUsesMarkdownLinks(t *testing.T) {
	cwd := t.TempDir()
	linked := filepath.Join(cwd, "linked.txt")
	backtickOnly := filepath.Join(cwd, "backtick.txt")
	if err := os.WriteFile(linked, []byte("linked"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backtickOnly, []byte("backtick"), 0o644); err != nil {
		t.Fatal(err)
	}

	answer := "Created [linked.txt](linked.txt).\nAlso mentioned `" + backtickOnly + "`."
	paths := finalAnswerFilePaths(cwd, answer)

	if len(paths) != 1 {
		t.Fatalf("expected one linked path, got %d: %#v", len(paths), paths)
	}
	if paths[0] != filepath.Clean(linked) {
		t.Fatalf("expected %q, got %q", filepath.Clean(linked), paths[0])
	}
}

func TestFinalAnswerFilePathsStripsLineReferences(t *testing.T) {
	cwd := t.TempDir()
	source := filepath.Join(cwd, "source.go")
	if err := os.WriteFile(source, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths := finalAnswerFilePaths(cwd, "See [source.go](source.go:12).")

	if len(paths) != 1 {
		t.Fatalf("expected one path, got %d: %#v", len(paths), paths)
	}
	if paths[0] != filepath.Clean(source) {
		t.Fatalf("expected %q, got %q", filepath.Clean(source), paths[0])
	}
}

func TestFinalAnswerFilePathsSkipsRemoteLinksAndMissingFiles(t *testing.T) {
	cwd := t.TempDir()

	paths := finalAnswerFilePaths(cwd, "[remote](https://example.com/file.txt) [missing](missing.txt)")

	if len(paths) != 0 {
		t.Fatalf("expected no paths, got %#v", paths)
	}
}
