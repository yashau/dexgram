package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintVersion(t *testing.T) {
	var buf bytes.Buffer
	printVersion(&buf)

	want := "Dexgram " + appVersion + "\n"
	if got := buf.String(); got != want {
		t.Fatalf("printVersion() = %q, want %q", got, want)
	}
}

func TestPrintServiceStatusHeaderIncludesVersion(t *testing.T) {
	var buf bytes.Buffer
	printServiceStatusHeader(&buf)

	if !strings.Contains(buf.String(), "version: "+appVersion+"\n") {
		t.Fatalf("service status header did not include version: %q", buf.String())
	}
}
