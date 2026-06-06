package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDayRange(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	// Default 08:00 start: window spans 08:00 of the label day to 07:59:59 next.
	cfg := config{Day: time.Date(2026, 6, 5, 0, 0, 0, 0, jst), Location: jst, StartHour: 8}
	want := "2026-06-05T08:00:00+09:00..2026-06-06T07:59:59+09:00"
	if got := cfg.dayRange(); got != want {
		t.Errorf("dayRange() startHour=8 = %q, want %q", got, want)
	}

	// Midnight start (StartHour 0) keeps the calendar-day window.
	midnight := config{Day: time.Date(2026, 6, 5, 0, 0, 0, 0, jst), Location: jst, StartHour: 0}
	wantMidnight := "2026-06-05T00:00:00+09:00..2026-06-05T23:59:59+09:00"
	if got := midnight.dayRange(); got != wantMidnight {
		t.Errorf("dayRange() startHour=0 = %q, want %q", got, wantMidnight)
	}
}

func TestParseHour(t *testing.T) {
	for _, in := range []string{"0", "8", " 23 "} {
		if _, err := parseHour(in); err != nil {
			t.Errorf("parseHour(%q) unexpected error: %v", in, err)
		}
	}
	for _, in := range []string{"-1", "24", "x", ""} {
		if _, err := parseHour(in); err == nil {
			t.Errorf("parseHour(%q) expected error, got nil", in)
		}
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	cases := []struct {
		in   string
		want string
	}{
		{"~", home},
		{"~/Documents/vault", filepath.Join(home, "Documents", "vault")},
		{`~\Documents\vault`, filepath.Join(home, "Documents", "vault")},
		{"/abs/path", "/abs/path"},
		{"relative/path", "relative/path"},
		{"~user/other", "~user/other"}, // ~user is left untouched
	}
	for _, c := range cases {
		got, err := expandHome(c.in)
		if err != nil {
			t.Errorf("expandHome(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("expandHome(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRepoFromAPIURL(t *testing.T) {
	cases := map[string]string{
		"https://api.github.com/repos/owner/name": "owner/name",
		"https://api.github.com/repos/a/b/c":       "a/b/c",
		"no-marker-here":                           "no-marker-here",
	}
	for in, want := range cases {
		if got := repoFromAPIURL(in); got != want {
			t.Errorf("repoFromAPIURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("  hello  ", 10); got != "hello" {
		t.Errorf("truncate trim = %q", got)
	}
	if got := truncate("abcdef", 3); got != "abc…" {
		t.Errorf("truncate cut = %q", got)
	}
}
