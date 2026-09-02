package text

import (
	"unicode/utf8"
)

// CapRunes caps s to n runes without breaking multi-byte UTF-8 sequences.
func CapRunes(s string, n int) string {
	if n < 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// IsWordByte reports whether byte c is an ASCII alphanumeric character or underscore.
func IsWordByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

// First10 returns the first 10 characters of s (e.g. YYYY-MM-DD from an ISO timestamp).
func First10(s string) string {
	if len(s) < 10 {
		return s
	}
	return s[:10]
}

// Sid8 returns the first 8 runes of a session ID for concise display.
func Sid8(sessionID string) string {
	return CapRunes(sessionID, 8)
}
