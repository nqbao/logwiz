package logmsg

import (
	"regexp"
	"testing"
)

var fullTsRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}$`)

func TestNormalizeTime_RFC3339Nano(t *testing.T) {
	got := NormalizeTime("2026-04-21T14:02:31.123Z")
	if !fullTsRe.MatchString(got) {
		t.Errorf("NormalizeTime(RFC3339Nano) = %q, want YYYY-MM-DD HH:MM:SS.mmm format", got)
	}
	if got != "2026-04-21 14:02:31.123" {
		t.Errorf("NormalizeTime(RFC3339Nano UTC) = %q, want \"2026-04-21 14:02:31.123\"", got)
	}
}

func TestNormalizeTime_RFC3339(t *testing.T) {
	got := NormalizeTime("2026-04-21T14:02:31Z")
	if !fullTsRe.MatchString(got) {
		t.Errorf("NormalizeTime(RFC3339) = %q, want YYYY-MM-DD HH:MM:SS.mmm format", got)
	}
	if got != "2026-04-21 14:02:31.000" {
		t.Errorf("NormalizeTime(RFC3339 UTC) = %q, want \"2026-04-21 14:02:31.000\"", got)
	}
}

func TestNormalizeTime_EpochSec(t *testing.T) {
	got := NormalizeTime("1745234551")
	if !fullTsRe.MatchString(got) {
		t.Errorf("NormalizeTime(epoch-sec) = %q, want YYYY-MM-DD HH:MM:SS.mmm format", got)
	}
}

func TestNormalizeTime_EpochMs(t *testing.T) {
	got := NormalizeTime("1745234551123")
	if !fullTsRe.MatchString(got) {
		t.Errorf("NormalizeTime(epoch-ms) = %q, want YYYY-MM-DD HH:MM:SS.mmm format", got)
	}
}

func TestNormalizeTime_EpochSecFloat(t *testing.T) {
	// 1234567890.123 → 2009-02-13 23:31:30.123
	got := NormalizeTime("1234567890.123")
	if !fullTsRe.MatchString(got) {
		t.Errorf("NormalizeTime(epoch-sec float) = %q, want YYYY-MM-DD HH:MM:SS.mmm format", got)
	}
	if got != "2009-02-13 23:31:30.123" {
		t.Errorf("NormalizeTime(epoch-sec float) = %q, want \"2009-02-13 23:31:30.123\"", got)
	}
}

func TestNormalizeTime_Unparseable(t *testing.T) {
	const s = "not-a-timestamp"
	if got := NormalizeTime(s); got != s {
		t.Errorf("NormalizeTime(unparseable) = %q, want unchanged %q", got, s)
	}
}

func TestNormalizeLevel_Aliases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"info", "INFO "},
		{"INFO", "INFO "},
		{"warn", "WARN "},
		{"warning", "WARN "},
		{"WARNING", "WARN "},
		{"err", "ERROR"},
		{"error", "ERROR"},
		{"ERR", "ERROR"},
		{"fatal", "FATAL"},
		{"panic", "FATAL"},
		{"debug", "DEBUG"},
		{"trace", "TRACE"},
	}
	for _, tc := range cases {
		if got := NormalizeLevel(tc.in); got != tc.want {
			t.Errorf("NormalizeLevel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeLevel_Padding(t *testing.T) {
	got := NormalizeLevel("ok")
	if len(got) != 5 {
		t.Errorf("NormalizeLevel(\"ok\") len = %d, want 5; got %q", len(got), got)
	}
}

func TestQuoteIfNeeded(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", `""`},
		{"simple", "simple"},
		{"123", "123"},
		{"has space", `"has space"`},
		{"has=equals", `"has=equals"`},
		{"has\"quote", `"has\"quote"`},
		{"has'apostrophe", `"has'apostrophe"`},
	}
	for _, tc := range cases {
		if got := QuoteIfNeeded(tc.in); got != tc.want {
			t.Errorf("QuoteIfNeeded(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
