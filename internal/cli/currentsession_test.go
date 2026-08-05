package cli

import "testing"

// TestCurrentSessionResolution: the flag is the contract, but the env fallback is
// what makes the exclusion actually fire — an agent running `rawclaw "query"` in
// a shell does not know it just typed something, and will not pass a flag. "off"
// is the way back to searching your own live turn.
func TestCurrentSessionResolution(t *testing.T) {
	tests := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{name: "flag wins", flag: "aaaa1111", env: "bbbb2222", want: "aaaa1111"},
		{name: "env fallback", flag: "", env: "bbbb2222", want: "bbbb2222"},
		{name: "neither", flag: "", env: "", want: ""},
		{name: "off disables the env", flag: "off", env: "bbbb2222", want: ""},
		{name: "OFF is case-insensitive", flag: "OFF", env: "bbbb2222", want: ""},
		{name: "whitespace is not a session", flag: "  ", env: "  ", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(currentSessionEnv, tc.env)
			o := &Options{CurrentSession: tc.flag}
			if got := o.currentSession(); got != tc.want {
				t.Errorf("currentSession() = %q, want %q", got, tc.want)
			}
		})
	}
}
