package main

import (
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    int
	}{
		{current: "0.1.2", latest: "v0.1.3", want: -1},
		{current: "0.1.3", latest: "v0.1.3", want: 0},
		{current: "0.1.4", latest: "v0.1.3", want: 1},
		{current: "0.2.0", latest: "v0.1.9", want: 1},
	}
	for _, test := range tests {
		got, err := compareVersions(test.current, test.latest)
		if err != nil {
			t.Fatalf("compareVersions(%q, %q): %v", test.current, test.latest, err)
		}
		if got != test.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.current, test.latest, got, test.want)
		}
	}
}

func TestVersionPartsAcceptsTwoOrThreePartsAndMetadata(t *testing.T) {
	got, err := versionParts("V1.2.3-beta+build ")
	if err != nil {
		t.Fatalf("version parts: %v", err)
	}
	if got != [3]int{1, 2, 3} {
		t.Fatalf("version parts = %#v", got)
	}

	got, err = versionParts("1.2")
	if err != nil {
		t.Fatalf("two-part version: %v", err)
	}
	if got != [3]int{1, 2, 0} {
		t.Fatalf("two-part version = %#v", got)
	}
}

func TestVersionPartsRejectsMalformedVersions(t *testing.T) {
	for _, version := range []string{"1", "1.2.3.4", "1.two.3"} {
		if _, err := versionParts(version); err == nil {
			t.Fatalf("versionParts(%q) succeeded, want error", version)
		}
	}
	if _, err := compareVersions("bad", "1.2.3"); err == nil {
		t.Fatal("compareVersions accepted malformed current version")
	}
}

func TestUpdatePowerShellScriptCarriesParentProcess(t *testing.T) {
	got := updatePowerShellScript(1234)
	for _, want := range []string{
		"$env:UPDATE='true'",
		"$env:DEXGRAM_UPDATE_PARENT_PID='1234'",
		installScriptURL,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("update script missing %q: %q", want, got)
		}
	}
}
