package archive

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strings"
)

// resolveTags picks the winning tagging for ONE session from all machines' tag
// files for it (the cross-machine union ingest resolves over). It is the unified
// resolver, and it is MACHINE-AGNOSTIC — it needs no "which machine is origin"
// lookup, which is exactly what lets it stay convergent for forked/dual-home
// sessions:
//
//   - Segments (the authored unit): the UNIQUE real segment set wins. In the
//     common path a session has exactly one tag file (its sole author = its
//     origin machine), so its set is trivially unique and wins — origin-authority
//     falls out for free. If ≥2 DISTINCT real sets exist (differing content),
//     that is a genuine conflict: pick a deterministic, content-independent
//     convergent winner — highest (origin_machine, content-hash) — and flag it so
//     the caller can surface it. Never wall-clock. Identical sets from
//     several machines are NOT a conflict (same content-hash).
//   - "Real beats routine": a file with real segments outranks a routine/empty
//     one, so a routine verdict never suppresses a real tagging.
//   - Verdict (routine): resolved separately by a fixed tie rule — latest
//     tagged_at wins, tie → higher origin_machine (cosmetic; both say routine).
//
// The loser's bytes are never lost: each machine's tag file persists in its own
// archive dir; resolveTags only decides which set the index surfaces.
func resolveTags(files []TagFile) (segs []TagSegment, segOrigin string, verdict *TagVerdict, verdictOrigin string, conflict bool) {
	segs, segOrigin, conflict = resolveSegments(files)
	verdict, verdictOrigin = resolveVerdict(files)
	return segs, segOrigin, verdict, verdictOrigin, conflict
}

// resolveSegments implements the segment half of resolveTags. Returns the winning
// set (nil if no machine tagged real segments), the winner's origin machine, and
// whether the win resolved a real conflict (≥2 distinct real sets).
func resolveSegments(files []TagFile) ([]TagSegment, string, bool) {
	type real struct {
		file TagFile
		hash string
	}
	var reals []real
	distinct := map[string]struct{}{}
	for _, f := range files {
		if !hasRealSegment(f.Segments) {
			continue // routine/empty file: cannot win the segment slot
		}
		h := segHash(f.Segments)
		reals = append(reals, real{file: f, hash: h})
		distinct[h] = struct{}{}
	}
	if len(reals) == 0 {
		return nil, "", false
	}
	if len(distinct) == 1 {
		// One authored set (possibly replicated identically on several machines).
		// Not a conflict — but the ATTRIBUTION must still be deterministic: taking
		// reals[0] would credit whichever machine gatherTagFiles happened to list
		// first (own dir + alphabetical foreign dirs), so the origin_machine column
		// would diverge across boxes even though the content agrees. Pick the
		// highest origin_machine among the identical-content files — order-
		// independent, converges everywhere.
		best := reals[0]
		for _, r := range reals[1:] {
			if r.file.OriginMachine > best.file.OriginMachine {
				best = r
			}
		}
		return best.file.Segments, best.file.OriginMachine, false
	}
	// ≥2 distinct real sets: a genuine conflict. Deterministic convergent winner:
	// highest (origin_machine, content-hash). Every machine computes the same one.
	best := reals[0]
	for _, r := range reals[1:] {
		if r.file.OriginMachine > best.file.OriginMachine ||
			(r.file.OriginMachine == best.file.OriginMachine && r.hash > best.hash) {
			best = r
		}
	}
	return best.file.Segments, best.file.OriginMachine, true
}

// resolveVerdict picks the winning verdict across machines by a fixed tie
// rule: latest tagged_at wins, tie → higher origin_machine. Returns nil when no
// machine wrote a verdict. Low-stakes: the only verdict kind is routine, so the
// outcome is identical and the tie-break only selects attribution.
func resolveVerdict(files []TagFile) (*TagVerdict, string) {
	var best *TagVerdict
	var bestOrigin string
	for _, f := range files {
		if f.Verdict == nil {
			continue
		}
		v := f.Verdict
		switch {
		case best == nil,
			v.TaggedAt > best.TaggedAt,
			v.TaggedAt == best.TaggedAt && f.OriginMachine > bestOrigin:
			vv := *v
			best = &vv
			bestOrigin = f.OriginMachine
		}
	}
	return best, bestOrigin
}

// hasRealSegment reports whether a segment set carries at least one non-empty
// topic — the "real tag" signal that outranks a routine/empty file.
func hasRealSegment(segs []TagSegment) bool {
	for _, s := range segs {
		if strings.TrimSpace(s.Topic) != "" {
			return true
		}
	}
	return false
}

// segHash is a content hash of a segment set, order-independent (segments are
// sorted by start uuid first), so two machines that authored the same set — in
// any row order — collide and are not treated as a conflict. Covers the fields a
// human would call "the tagging": boundaries + topic + summary.
func segHash(segs []TagSegment) string {
	sorted := make([]TagSegment, len(segs))
	copy(sorted, segs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartUUID < sorted[j].StartUUID })
	h := sha1.New()
	for _, s := range sorted {
		// NUL-delimit so field boundaries can't be forged by content.
		h.Write([]byte(s.StartUUID + "\x00" + s.EndUUID + "\x00" + s.Topic + "\x00" + s.Summary + "\x00"))
	}
	return hex.EncodeToString(h.Sum(nil))
}
