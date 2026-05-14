package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"dexgram/internal/state"
)

const (
	telegramPairingCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	telegramPairingCodeLength   = 6
	telegramPairingCodeTTL      = 10 * time.Minute
)

func createTelegramPairingCode(store *state.Store, chatID int64) (string, error) {
	var lastErr error
	for range 8 {
		code, err := generateTelegramPairingCode()
		if err != nil {
			return "", err
		}
		if err := store.SaveTelegramPairingCode(code, chatID, time.Now().Add(telegramPairingCodeTTL)); err != nil {
			lastErr = err
			continue
		}
		return code, nil
	}
	return "", fmt.Errorf("save Telegram pairing code: %w", lastErr)
}

func generateTelegramPairingCode() (string, error) {
	var b strings.Builder
	b.Grow(telegramPairingCodeLength)
	max := big.NewInt(int64(len(telegramPairingCodeAlphabet)))
	for range telegramPairingCodeLength {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate Telegram pairing code: %w", err)
		}
		b.WriteByte(telegramPairingCodeAlphabet[n.Int64()])
	}
	return b.String(), nil
}

func formatTelegramPairingCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != telegramPairingCodeLength {
		return code
	}
	return code[:3] + "-" + code[3:]
}

func normalizeTelegramPairingCode(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) == 7 {
		if value[3] != '-' {
			return "", fmt.Errorf("invalid Telegram pairing code %q; expected XXX-XXX or XXXXXX", value)
		}
		value = value[:3] + value[4:]
	}
	if len(value) != telegramPairingCodeLength {
		return "", fmt.Errorf("invalid Telegram pairing code %q; expected XXX-XXX or XXXXXX", value)
	}
	for _, r := range value {
		if !strings.ContainsRune(telegramPairingCodeAlphabet, r) {
			return "", fmt.Errorf("invalid Telegram pairing code %q; expected letters or digits from the pairing code", value)
		}
	}
	return value, nil
}
