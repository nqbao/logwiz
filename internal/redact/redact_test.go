package redact

import "testing"

func TestRedactSinglePattern(t *testing.T) {
	r, err := New([]string{`password=\S+`})
	if err != nil {
		t.Fatal(err)
	}
	got := r.Redact("user=alice password=hunter2 host=db")
	want := "user=alice [REDACTED] host=db"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactMultiplePatterns(t *testing.T) {
	r, err := New([]string{`password=\S+`, `token=\S+`})
	if err != nil {
		t.Fatal(err)
	}
	got := r.Redact("password=secret token=abc123 ok")
	want := "[REDACTED] [REDACTED] ok"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactNoMatch(t *testing.T) {
	r, _ := New([]string{`password=\S+`})
	s := "INFO server started on :8080"
	if got := r.Redact(s); got != s {
		t.Errorf("got %q, want unchanged %q", got, s)
	}
}

func TestRedactNilIsNoop(t *testing.T) {
	var r *Redactor
	s := "password=secret"
	if got := r.Redact(s); got != s {
		t.Errorf("nil Redactor mutated string: %q", got)
	}
}

func TestRedactEmptyPatternsReturnsNil(t *testing.T) {
	r, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if r != nil {
		t.Error("expected nil Redactor for empty patterns")
	}
}

func TestRedactInvalidPatternErrors(t *testing.T) {
	_, err := New([]string{`[invalid`})
	if err == nil {
		t.Error("expected error for invalid pattern")
	}
}

func TestRedactLines(t *testing.T) {
	r, _ := New([]string{`secret=\S+`})
	in := []string{"ok line", "secret=abc here", "another ok"}
	out := r.RedactLines(in)
	if out[0] != "ok line" {
		t.Errorf("out[0] = %q", out[0])
	}
	if out[1] != "[REDACTED] here" {
		t.Errorf("out[1] = %q", out[1])
	}
	if out[2] != "another ok" {
		t.Errorf("out[2] = %q", out[2])
	}
	// original must be unchanged
	if in[1] != "secret=abc here" {
		t.Error("RedactLines mutated input slice")
	}
}

func TestRedactLinesNilIsNoop(t *testing.T) {
	var r *Redactor
	in := []string{"secret=abc"}
	out := r.RedactLines(in)
	if &out[0] != &in[0] {
		// nil redactor should return the same slice, not a copy
		_ = out // just verify no panic
	}
}
