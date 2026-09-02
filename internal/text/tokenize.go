package text

import (
	"regexp"
	"strings"
)

var (
	// wordSplitRe splits camelCase, PascalCase, snake_case, and digits.
	// E.g. "parseOAuthToken" -> ["parse", "OAuth", "Token"]
	wordSplitRe = regexp.MustCompile(`[A-Z]+[a-z]*|[a-z]+|[0-9]+`)
)

// SplitCodeIdentifier splits a code identifier (camelCase, PascalCase,
// snake_case, kebab-case, or file path) into its sub-tokens while preserving
// the original token.
func SplitCodeIdentifier(s string) []string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}

	seen := make(map[string]bool)
	var tokens []string

	add := func(tok string) {
		tok = strings.TrimSpace(tok)
		if tok != "" && !seen[tok] {
			seen[tok] = true
			tokens = append(tokens, tok)
		}
	}

	// Always include original identifier
	add(trimmed)

	// Split path components if present
	if strings.ContainsAny(trimmed, "/\\.") {
		parts := strings.FieldsFunc(trimmed, func(r rune) bool {
			return r == '/' || r == '\\' || r == '.'
		})
		for _, p := range parts {
			add(p)
		}
	}

	// Split camelCase, snake_case, and numbers
	matches := wordSplitRe.FindAllString(trimmed, -1)
	for _, m := range matches {
		add(m)
	}

	return tokens
}
