package main

import "testing"

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
