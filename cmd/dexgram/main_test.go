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
