package main

import (
	"reflect"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTurnControlMarkup(t *testing.T) {
	if turnControlMarkup("", false) != nil {
		t.Fatal("expected empty token to produce no markup")
	}

	queued := turnControlMarkup("abc", true)
	if queued == nil || len(queued.InlineKeyboard) != 1 || len(queued.InlineKeyboard[0]) != 2 {
		t.Fatalf("unexpected queued markup: %#v", queued)
	}
	if queued.InlineKeyboard[0][0].CallbackData != "st:abc" {
		t.Fatalf("unexpected steer callback: %#v", queued.InlineKeyboard[0][0])
	}
	if queued.InlineKeyboard[0][1].CallbackData != "dq:abc" {
		t.Fatalf("unexpected delete callback: %#v", queued.InlineKeyboard[0][1])
	}

	active := turnControlMarkup("xyz", false)
	if active == nil || active.InlineKeyboard[0][0].CallbackData != "sp:xyz" {
		t.Fatalf("unexpected active markup: %#v", active)
	}
}

func TestTelegramUtilityHelpers(t *testing.T) {
	if emptyAs("  ", "fallback") != "fallback" {
		t.Fatal("emptyAs did not return fallback")
	}
	if emptyAs(" value ", "fallback") != " value " {
		t.Fatal("emptyAs should preserve non-empty value")
	}

	got := splitNonEmptyLines(" one \n\n two \n   \nthree")
	want := []string{"one", "two", "three"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitNonEmptyLines = %#v, want %#v", got, want)
	}
}

func TestMessageHasAttachment(t *testing.T) {
	if messageHasAttachment(&models.Message{}) {
		t.Fatal("empty message should not have attachments")
	}
	if !messageHasAttachment(&models.Message{Photo: []models.PhotoSize{{FileID: "photo"}}}) {
		t.Fatal("photo should count as attachment")
	}
	if !messageHasAttachment(&models.Message{Document: &models.Document{FileID: "doc"}}) {
		t.Fatal("document should count as attachment")
	}
	if !messageHasAttachment(&models.Message{Video: &models.Video{FileID: "video"}}) {
		t.Fatal("video should count as attachment")
	}
}
