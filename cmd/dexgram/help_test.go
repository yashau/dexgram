package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestPrintHelpIncludesCoreCommandsAndFlagDefaults(t *testing.T) {
	var out bytes.Buffer
	fs := flag.NewFlagSet("dexgram", flag.ContinueOnError)
	fs.SetOutput(&out)
	fs.String("config", "", "path to config")
	fs.Bool("check", false, "validate setup")

	printHelp(&out, "dexgram.exe", fs)

	text := out.String()
	for _, want := range []string{
		"Dexgram",
		"dexgram.exe -check",
		"dexgram.exe telegram chatid add <chat_id_or_pairing_code>",
		"dexgram.exe service <install|start|stop|restart|status|uninstall>",
		"-config string",
		"Local state:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("help output missing %q:\n%s", want, text)
		}
	}
}
