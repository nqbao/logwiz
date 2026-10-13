package logmsg

import (
	"testing"
	"time"
)

func TestPipeParser_Header(t *testing.T) {
	p := &PipeParser{}
	raw := "2024-01-02 15:04:05,123 | INFO  | main | com.example.App | Started in 1.2s | ok"
	ms := p.Parse(raw)
	if len(ms) != 1 {
		t.Fatalf("got %d messages", len(ms))
	}
	m := ms[0]
	if m.Raw != raw || m.Format != "pipe" {
		t.Errorf("Raw/Format = %q/%q", m.Raw, m.Format)
	}
	if m.Time != "2024-01-02 15:04:05,123" || m.Level != "INFO " {
		t.Errorf("Time/Level = %q/%q", m.Time, m.Level)
	}
	if m.Message != "Started in 1.2s | ok" {
		t.Errorf("Message = %q", m.Message)
	}
	if m.Extra.Get("thread") != "main" || m.Extra.Get("logger") != "com.example.App" {
		t.Errorf("thread/logger = %q/%q", m.Extra.Get("thread"), m.Extra.Get("logger"))
	}
}

func TestPipeParser_RejectsNonTimestamp(t *testing.T) {
	p := &PipeParser{}
	for _, raw := range []string{"foo | bar | baz", "no pipes here", "2024-01-02 15:04:05 no pipe"} {
		if m := p.Parse(raw)[0]; m.Format != "" {
			t.Errorf("%q parsed as %q", raw, m.Format)
		}
	}
}

func TestPipeParser_StackTraceInheritsLevel(t *testing.T) {
	p := &PipeParser{}
	lines := []string{
		"2024-01-02 15:04:05.000 | ERROR | worker-1 | com.example.Job | boom",
		"java.lang.IllegalStateException: bad",
		"\tat com.example.Job.run(Job.java:42)",
		"Caused by: java.io.IOException: eof",
		"\t... 3 more",
		"plain line",
		"\tat not.a.continuation(After.java:1)",
	}
	var got []LogMessage
	for _, l := range lines {
		got = append(got, p.Parse(l)...)
	}
	for i := 1; i <= 4; i++ {
		if got[i].Level != "ERROR" || got[i].Time != "2024-01-02 15:04:05.000" || got[i].Format != "" || got[i].Raw != lines[i] {
			t.Errorf("line %d = %+v, want inherited ERROR passthrough", i, got[i])
		}
	}
	if got[5].Level != "" || got[6].Level != "" {
		t.Errorf("non-continuation lines inherited level: %+v %+v", got[5], got[6])
	}
}

func TestPipeParser_StrictContinuation(t *testing.T) {
	p := &PipeParser{Strict: true}
	p.Parse("12:00:00 | WARN | slow request")
	if m := p.Parse("any follow-up text")[0]; m.Level != "WARN " {
		t.Errorf("strict continuation Level = %q", m.Level)
	}
}

func TestAutoParser_DetectsPipe(t *testing.T) {
	p := &AutoParser{}
	if m := p.Parse("2024-01-02T15:04:05Z | DEBUG | hello world")[0]; m.Format != "pipe" || m.Message != "hello world" {
		t.Errorf("got %+v", m)
	}
	if m := p.Parse("\tat x.Y(Y.java:1)")[0]; m.Level != "DEBUG" {
		t.Errorf("continuation Level = %q", m.Level)
	}
	p.Parse(`{"msg":"json"}`)
	if m := p.Parse("\tat x.Y(Y.java:1)")[0]; m.Level != "" {
		t.Errorf("continuation after JSON inherited Level %q", m.Level)
	}
}

func TestParseTime_ZonelessLocal(t *testing.T) {
	got, ok := ParseTime("2024-01-02 15:04:05,123")
	want := time.Date(2024, 1, 2, 15, 4, 5, 123e6, time.Local)
	if !ok || !got.Equal(want) {
		t.Errorf("ParseTime = %v %v, want %v", got, ok, want)
	}
}
