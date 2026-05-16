package main

import (
	"fmt"
	"testing"

	"github.com/go-telegram/bot"
)

func TestIsTelegramTopicGoneError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{err: fmt.Errorf("%w, Bad Request: TOPIC_DELETED", bot.ErrorBadRequest), want: true},
		{err: fmt.Errorf("%w, Bad Request: message thread not found", bot.ErrorBadRequest), want: true},
		{err: fmt.Errorf("%w, Bad Request: chat not found", bot.ErrorBadRequest), want: false},
		{err: fmt.Errorf("Bad Request: TOPIC_DELETED"), want: false},
	}

	for _, test := range tests {
		if got := isTelegramTopicGoneError(test.err); got != test.want {
			t.Fatalf("isTelegramTopicGoneError(%v) = %v, want %v", test.err, got, test.want)
		}
	}
}
