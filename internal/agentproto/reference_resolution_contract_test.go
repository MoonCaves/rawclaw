package agentproto

import (
	"errors"
	"testing"
)

func TestReadNumericReferenceContract(t *testing.T) {
	proj := t.TempDir()
	writeMultiSession(t, proj, "12345678aaaa", [][2]string{
		{"108364040001", "all digit prefix"},
		{"900000000001", "second ordinal"},
		{"42abcdef0001", "forty two prefix"},
	})
	writeSession(t, proj, "0123abcd1111", "0123abcd0001", "numeric leading prefix")
	scope := scopeFor(t, proj)

	for _, tc := range []struct {
		name, ref, session, text string
	}{
		{"all digit prefixes", "12345678:10836404", "12345678aaaa", "all digit prefix"},
		{"numeric leading prefixes", "0123abcd:0123abcd", "0123abcd1111", "numeric leading prefix"},
		{"printed short numeric prefix", "ref=12345678:42", "12345678aaaa", "forty two prefix"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Read(tc.ref, scope, ReadOpts{})
			if err != nil {
				t.Fatalf("Read(%q): %v", tc.ref, err)
			}
			if got.SessionID != tc.session {
				t.Fatalf("SessionID = %q, want %q", got.SessionID, tc.session)
			}
			anchors := 0
			for _, msg := range got.Window {
				if !msg.Anchor {
					continue
				}
				anchors++
				if msg.Text != tc.text {
					t.Errorf("anchor text = %q, want %q", msg.Text, tc.text)
				}
			}
			if anchors != 1 {
				t.Fatalf("marked anchors = %d, want 1", anchors)
			}
		})
	}

	t.Run("ordinal does not resolve", func(t *testing.T) {
		_, err := Read("12345678:2", scope, ReadOpts{})
		var missing *ErrMsgNotFound
		if !errors.As(err, &missing) {
			t.Fatalf("Read ordinal error = %v, want *ErrMsgNotFound", err)
		}
		if missing.UUID8 != "2" {
			t.Fatalf("missing UUID8 = %q, want 2", missing.UUID8)
		}
	})
}
