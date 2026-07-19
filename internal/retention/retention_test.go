package retention

import "testing"

// TestDecideRetention_ReplicaAbsenceAuthoritative pins the replica axis of
// the shared decision tree: in an ARCHIVE-replica scope the scanned tree (the
// synced clone) is the source of truth, so an absent file prunes its rows —
// regardless of tombstones, ownership, or the mirror setting — while a
// present file behaves exactly as in a local scope. The local-scope rows
// (replica=false) pin that D1/D2/D5 and the mirror opt-out are untouched.
func TestDecideRetention_ReplicaAbsenceAuthoritative(t *testing.T) {
	tests := []struct {
		name                                                  string
		present, tombstoned, own, missingSet, mirror, replica bool
		want                                                  RetentionAction
	}{
		// Replica scope: absence = propagated delete (E5) → prune.
		{"replica absent foreign", false, false, false, false, false, true, ActPrune},
		{"replica absent foreign flagged", false, false, false, true, false, true, ActPrune},
		{"replica absent tombstoned", false, true, false, false, false, true, ActPrune},
		{"replica absent own-row oddity", false, false, true, false, false, true, ActPrune},
		{"replica absent mirror set", false, false, false, false, true, true, ActPrune},
		// Replica scope: presence behaves as anywhere else.
		{"replica present", true, false, false, false, false, true, ActNone},
		{"replica present clears stale flag", true, false, false, true, false, true, ActClear},
		// Local scopes unchanged: D1 stamp, D2 foreign skip, D5 tombstone,
		// mirror opt-out.
		{"local absent own stamps (D1)", false, false, true, false, false, false, ActStamp},
		{"local absent own already flagged", false, false, true, true, false, false, ActNone},
		{"local absent foreign skipped (D2)", false, false, false, false, false, false, ActNone},
		{"local absent tombstoned prunes (D5)", false, true, true, false, false, false, ActPrune},
		{"local absent own mirror prunes", false, false, true, false, true, false, ActPrune},
		{"local present", true, false, true, false, false, false, ActNone},
		{"local present clears stale flag", true, false, true, true, false, false, ActClear},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideRetention(tt.present, tt.tombstoned, tt.own, tt.missingSet, tt.mirror, tt.replica)
			if got != tt.want {
				t.Errorf("DecideRetention(present=%v tombstoned=%v own=%v missingSet=%v mirror=%v replica=%v) = %v, want %v",
					tt.present, tt.tombstoned, tt.own, tt.missingSet, tt.mirror, tt.replica, got, tt.want)
			}
		})
	}
}
