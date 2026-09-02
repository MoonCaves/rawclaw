package text_test

import (
	"testing"

	"github.com/MoonCaves/rawclaw/internal/text"
)

func TestCapRunes(t *testing.T) {
	if got := text.CapRunes("hello world", 5); got != "hello" {
		t.Errorf("CapRunes = %q, want 'hello'", got)
	}
	if got := text.CapRunes("日本語", 2); got != "日本" {
		t.Errorf("CapRunes multi-byte = %q, want '日本'", got)
	}
	if got := text.CapRunes("short", 10); got != "short" {
		t.Errorf("CapRunes short = %q, want 'short'", got)
	}
}

func TestIsWordByte(t *testing.T) {
	if !text.IsWordByte('a') || !text.IsWordByte('Z') || !text.IsWordByte('9') || !text.IsWordByte('_') {
		t.Errorf("IsWordByte expected true for alphanumeric and underscore")
	}
	if text.IsWordByte(' ') || text.IsWordByte('-') || text.IsWordByte('.') {
		t.Errorf("IsWordByte expected false for non-word bytes")
	}
}

func TestFirst10(t *testing.T) {
	if got := text.First10("2026-09-03T01:00:00Z"); got != "2026-09-03" {
		t.Errorf("First10 = %q, want '2026-09-03'", got)
	}
	if got := text.First10("short"); got != "short" {
		t.Errorf("First10 short = %q, want 'short'", got)
	}
}

func TestSid8(t *testing.T) {
	if got := text.Sid8("547be07f-df0c-4080-aa0f-1d5e607e43bf"); got != "547be07f" {
		t.Errorf("Sid8 = %q, want '547be07f'", got)
	}
	if got := text.Sid8("abc"); got != "abc" {
		t.Errorf("Sid8 short = %q, want 'abc'", got)
	}
	if got := text.Sid8("αβγδεζηθικλμ"); got != "αβγδεζηθ" {
		t.Errorf("Sid8 Unicode = %q, want 'αβγδεζηθ'", got)
	}
}
