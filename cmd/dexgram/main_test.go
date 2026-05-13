package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFatalLogPathFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "separate flag",
			args: []string{"-config", "dexgram.toml", "-log", `C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.log`},
			want: `C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.log`,
		},
		{
			name: "equals flag",
			args: []string{"-log=service.log"},
			want: "service.log",
		},
		{
			name: "double dash flag",
			args: []string{"--log", "service.log"},
			want: "service.log",
		},
		{
			name: "missing value",
			args: []string{"-log"},
			want: "",
		},
		{
			name: "absent",
			args: []string{"service", "status"},
			want: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := fatalLogPathFromArgs(test.args); got != test.want {
				t.Fatalf("fatalLogPathFromArgs() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAppendFatalLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Dexgram", "dexgram.log")

	appendFatalLog(path, "fatal error: boom\n")

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fatal log: %v", err)
	}
	if !strings.Contains(string(got), "fatal error: boom\n") {
		t.Fatalf("fatal log did not contain message: %q", got)
	}
}

func TestTrimLogFileLinesKeepsNewestLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dexgram.log")
	before := "old one\nold two\nnew one\nnew two\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	if err := trimLogFileLines(path, 2); err != nil {
		t.Fatalf("trim log: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if string(got) != "new one\nnew two\n" {
		t.Fatalf("trimmed log = %q", got)
	}
}

func TestTrimLogFileLinesKeepsFinalLineWithoutNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dexgram.log")
	if err := os.WriteFile(path, []byte("old\nnew"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	if err := trimLogFileLines(path, 1); err != nil {
		t.Fatalf("trim log: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("trimmed log = %q", got)
	}
}

func TestLineLimitedLogFileTrimsAfterWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dexgram.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	f, err := openLineLimitedLogFile(path, 3)
	if err != nil {
		t.Fatalf("open limited log: %v", err)
	}
	if _, err := f.Write([]byte("four\n")); err != nil {
		t.Fatalf("write limited log: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close limited log: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if string(got) != "two\nthree\nfour\n" {
		t.Fatalf("limited log = %q", got)
	}
}
