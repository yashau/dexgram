package main

import (
	"context"
	"testing"
	"time"
)

func TestTelegramQueueSerializesRequests(t *testing.T) {
	q := newTelegramQueue(20 * time.Millisecond)
	if err := q.Wait(context.Background(), "first", 1, 2); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if err := q.Wait(context.Background(), "second", 1, 2); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Fatalf("second wait returned too quickly: %s", elapsed)
	}
}

func TestTelegramQueueBackoff(t *testing.T) {
	q := newTelegramQueue(time.Millisecond)
	q.Backoff(20 * time.Millisecond)

	start := time.Now()
	if err := q.Wait(context.Background(), "after backoff", 1, 2); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Fatalf("backoff wait returned too quickly: %s", elapsed)
	}
}
