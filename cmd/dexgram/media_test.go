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

func TestFinalAnswerFilePathsTrimsAnglesQuotesAndDedupes(t *testing.T) {
	cwd := t.TempDir()
	report := filepath.Join(cwd, "report.txt")
	if err := os.WriteFile(report, []byte("report"), 0o644); err != nil {
		t.Fatal(err)
	}

	answer := `[one](<"report.txt">) [two]('report.txt')`
	paths := finalAnswerFilePaths(cwd, answer)

	if len(paths) != 1 {
		t.Fatalf("expected one deduped path, got %#v", paths)
	}
	if paths[0] != filepath.Clean(report) {
		t.Fatalf("expected %q, got %q", filepath.Clean(report), paths[0])
	}
}

func TestAttachmentInputAndLines(t *testing.T) {
	image := codexInputForAttachment(`C:\tmp\image.png`, "image")
	if image["type"] != "localImage" || image["path"] != `C:\tmp\image.png` {
		t.Fatalf("unexpected image input: %#v", image)
	}

	file := codexInputForAttachment(`C:\tmp\notes.txt`, "file")
	if file["type"] != "mention" || file["name"] != "notes.txt" || file["path"] != `C:\tmp\notes.txt` {
		t.Fatalf("unexpected file input: %#v", file)
	}

	if got := attachmentLine("image", `C:\tmp\image.png`); got != `Attached image: C:\tmp\image.png` {
		t.Fatalf("attachmentLine image = %q", got)
	}
	if got := attachmentLine("file", `C:\tmp\notes.txt`); got != `Attached file: C:\tmp\notes.txt` {
		t.Fatalf("attachmentLine file = %q", got)
	}
}

func TestFilenameAndImageHelpers(t *testing.T) {
	if got := safeFilename(` ..bad:/\name?.txt `); got != "bad___name_.txt" {
		t.Fatalf("safeFilename returned %q", got)
	}
	if got := safeFilename("..."); got != "file" {
		t.Fatalf("safeFilename empty fallback = %q", got)
	}
	if !isImagePath("PHOTO.JPEG") || isImagePath("archive.zip") {
		t.Fatal("isImagePath returned unexpected result")
	}
}
