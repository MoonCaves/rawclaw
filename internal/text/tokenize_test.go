package text

import (
	"reflect"
	"testing"
)

func TestSplitCodeIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{
			input: "parseOAuthToken",
			want:  []string{"parseOAuthToken", "parse", "OAuth", "Token"},
		},
		{
			input: "handle_user_request",
			want:  []string{"handle_user_request", "handle", "user", "request"},
		},
		{
			input: "internal/storage/sqlite.go",
			want:  []string{"internal/storage/sqlite.go", "internal", "storage", "sqlite", "go"},
		},
		{
			input: "API_v2_GetUserById",
			want:  []string{"API_v2_GetUserById", "API", "v", "2", "Get", "User", "By", "Id"},
		},
		{
			input: "",
			want:  nil,
		},
		{
			input: "simple",
			want:  []string{"simple"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := SplitCodeIdentifier(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SplitCodeIdentifier(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
