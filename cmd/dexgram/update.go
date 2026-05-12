package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const installScriptURL = "https://raw.githubusercontent.com/yashau/dexgram/main/install.ps1"
const latestReleaseURL = "https://api.github.com/repos/yashau/dexgram/releases/latest"

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func runUpdateCommand() error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	latest, err := latestReleaseTag(ctx)
	if err != nil {
		return err
	}
	cmp, err := compareVersions(appVersion, latest)
	if err != nil {
		return err
	}
	if cmp >= 0 {
		fmt.Printf("Dexgram is already up to date (%s).\n", appVersion)
		return nil
	}

	fmt.Printf("Updating Dexgram %s -> %s...\n", appVersion, strings.TrimPrefix(latest, "v"))
	script := fmt.Sprintf(
		"$env:UPDATE='true'; $env:DEXGRAM_UPDATE_PARENT_PID='%s'; irm %s | iex",
		strconv.Itoa(os.Getpid()),
		installScriptURL,
	)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func latestReleaseTag(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dexgram-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("check latest release: GitHub returned HTTP %d", resp.StatusCode)
	}
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	if strings.TrimSpace(release.TagName) == "" {
		return "", fmt.Errorf("check latest release: latest release has no tag")
	}
	return release.TagName, nil
}

func compareVersions(current, latest string) (int, error) {
	currentParts, err := versionParts(current)
	if err != nil {
		return 0, fmt.Errorf("parse current version %q: %w", current, err)
	}
	latestParts, err := versionParts(latest)
	if err != nil {
		return 0, fmt.Errorf("parse latest version %q: %w", latest, err)
	}
	for i := 0; i < 3; i++ {
		switch {
		case currentParts[i] > latestParts[i]:
			return 1, nil
		case currentParts[i] < latestParts[i]:
			return -1, nil
		}
	}
	return 0, nil
}

func versionParts(version string) ([3]int, error) {
	version = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(version, "v"), "V"))
	if before, _, ok := strings.Cut(version, "-"); ok {
		version = before
	}
	if before, _, ok := strings.Cut(version, "+"); ok {
		version = before
	}
	fields := strings.Split(version, ".")
	if len(fields) < 2 || len(fields) > 3 {
		return [3]int{}, fmt.Errorf("expected major.minor.patch")
	}
	var out [3]int
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil {
			return [3]int{}, err
		}
		out[i] = n
	}
	return out, nil
}
