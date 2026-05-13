package main

import (
	"context"
	"log"
	"sync"
	"time"
)

const telegramQueueMinInterval = 350 * time.Millisecond

var defaultTelegramQueue = newTelegramQueue(telegramQueueMinInterval)

type telegramQueue struct {
	mu          sync.Mutex
	minInterval time.Duration
	nextAt      time.Time
}

func newTelegramQueue(minInterval time.Duration) *telegramQueue {
	return &telegramQueue{minInterval: minInterval}
}

func waitTelegramQueue(ctx context.Context, op string, chatID int64, messageThreadID int) error {
	return defaultTelegramQueue.Wait(ctx, op, chatID, messageThreadID)
}

func backoffTelegramQueue(delay time.Duration) {
	defaultTelegramQueue.Backoff(delay)
}

func (q *telegramQueue) Wait(ctx context.Context, op string, chatID int64, messageThreadID int) error {
	for {
		wait := q.reserveDelay()
		if wait <= 0 {
			return nil
		}
		if wait > q.minInterval {
			log.Printf("telegram queue wait op=%q chat_id=%d thread_id=%d wait=%s", op, chatID, messageThreadID, wait.Round(time.Millisecond))
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (q *telegramQueue) reserveDelay() time.Duration {
	now := time.Now()
	q.mu.Lock()
	defer q.mu.Unlock()
	if now.Before(q.nextAt) {
		return q.nextAt.Sub(now)
	}
	q.nextAt = now.Add(q.minInterval)
	return 0
}

func (q *telegramQueue) Backoff(delay time.Duration) {
	if delay <= 0 {
		return
	}
	next := time.Now().Add(delay)
	q.mu.Lock()
	if q.nextAt.Before(next) {
		q.nextAt = next
	}
	q.mu.Unlock()
}
