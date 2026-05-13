package main

import "testing"

func TestTelegramCommandsIncludeUpdate(t *testing.T) {
	for _, command := range telegramCommands() {
		if command.Command == "update" {
			return
		}
	}
	t.Fatal("expected /update command to be registered")
}
