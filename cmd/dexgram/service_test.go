package main

import (
	"strings"
	"testing"
)

func TestStartupFallbackContentUsesCmdCompatibleQuotes(t *testing.T) {
	command := `"C:\Users\Yashau\AppData\Local\Dexgram\dexgram.exe" -config "C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.toml" -log "C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.log"`
	logPath := `C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.log`

	got := startupFallbackContent(command, logPath)

	if strings.Contains(got, `\"`) {
		t.Fatalf("startup fallback uses C-style quote escaping: %q", got)
	}
	wantLine := `start "" /min cmd.exe /d /c ""C:\Users\Yashau\AppData\Local\Dexgram\dexgram.exe" -config "C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.toml" -log "C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.log" >> "C:\Users\Yashau\AppData\Roaming\Dexgram\dexgram.log" 2>&1"`
	if !strings.Contains(got, wantLine+"\r\n") {
		t.Fatalf("startup fallback content = %q, want line %q", got, wantLine)
	}
}

func TestQuoteCmdArgDoublesEmbeddedQuotes(t *testing.T) {
	got := quoteCmdArg(`C:\Users\Name "Quoted"\dexgram.log`)
	want := `"C:\Users\Name ""Quoted""\dexgram.log"`
	if got != want {
		t.Fatalf("quoteCmdArg() = %q, want %q", got, want)
	}
}
