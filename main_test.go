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
	cfg := config{Day: time.Date(2026, 6, 5, 0, 0, 0, 0, jst), Location: jst}
	want := "2026-06-05T00:00:00+09:00..2026-06-05T23:59:59+09:00"
	if got := cfg.dayRange(); got != want {
		t.Errorf("dayRange() = %q, want %q", got, want)
	}

	utc := config{Day: time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC), Location: time.UTC}
	wantUTC := "2026-06-05T00:00:00Z..2026-06-05T23:59:59Z"
	if got := utc.dayRange(); got != wantUTC {
		t.Errorf("dayRange() UTC = %q, want %q", got, wantUTC)
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
