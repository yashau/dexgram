package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func TestPickTelegramAckReaction(t *testing.T) {
	want := []string{"🗿", "🌚", "👾", "🗿"}
	for i, reaction := range want {
		if got := pickTelegramAckReaction(uint64(i)); got != reaction {
			t.Fatalf("pickTelegramAckReaction(%d) = %q, want %q", i, got, reaction)
		}
	}
}

func TestTelegramEmojiReactionMarshals(t *testing.T) {
	reaction := telegramEmojiReaction("🗿")
	data, err := json.Marshal([]any{&reaction})
	if err != nil {
		t.Fatalf("marshal reaction: %v", err)
	}
	if got, want := string(data), `[{"type":"emoji","emoji":"🗿"}]`; got != want {
		t.Fatalf("reaction json = %s, want %s", got, want)
	}
}

func TestSetMessageReactionParamsEncodeEmojiReaction(t *testing.T) {
	client := &telegramAckCaptureClient{fields: map[string]string{}}
	b, err := bot.New(
		"token",
		bot.WithHTTPClient(time.Second, client),
		bot.WithServerURL("http://telegram.test"),
	)
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	_, err = b.SetMessageReaction(t.Context(), &bot.SetMessageReactionParams{
		ChatID:    int64(123),
		MessageID: 456,
		Reaction: []models.ReactionType{
			telegramEmojiReaction("🗿"),
		},
	})
	if err != nil {
		t.Fatalf("set reaction: %v", err)
	}
	if got, want := client.fields["reaction"], `[{"type":"emoji","emoji":"🗿"}]`; got != want {
		t.Fatalf("reaction form field = %s, want %s", got, want)
	}
}

type telegramAckCaptureClient struct {
	fields map[string]string
}

func (c *telegramAckCaptureClient) Do(req *http.Request) (*http.Response, error) {
	if strings.HasSuffix(req.URL.Path, "/getMe") {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Dexgram","username":"dexgram_bot"}}`)),
		}, nil
	}
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		return nil, err
	}
	for key, values := range req.MultipartForm.Value {
		if len(values) > 0 {
			c.fields[key] = values[0]
		}
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":true}`)),
	}, nil
}
