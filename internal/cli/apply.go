package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// The atomic replace-with-rollback below is adapted from github.com/minio/selfupdate
// (apply.go, Apache-2.0) — https://github.com/minio/selfupdate. We inline the ~40
// lines of the proven seam rather than carry the dependency: rawclaw layers its own
// mandatory sha256 verification (verifyChecksum) ahead of this, so the library's
// checksum/signature/patch machinery is dead weight. Credit and licence to MinIO.

// rollbackError reports that an update's final swap failed AND the automatic
// rollback to the original binary ALSO failed — the install is now in a bad state
// (no binary where the executable used to be) and needs a manual reinstall. A bare
// (non-rollbackError) error from applyTarget means the swap failed but the original
// binary is intact.
type rollbackError struct {
	commit   error // the error that made the swap fail
	rollback error // the error from the failed attempt to restore the original
}

func (e *rollbackError) Error() string {
	return fmt.Sprintf("update failed (%v) and rollback also failed (%v): original binary may be missing",
		e.commit, e.rollback)
}

func (e *rollbackError) Unwrap() error { return e.commit }

// asRollbackError returns the rollback error when err signals that the automatic
// rollback failed (the install needs manual recovery), else nil. Mirrors
// selfupdate.RollbackError: nil means the original binary is intact.
func asRollbackError(err error) error {
	var re *rollbackError
	if err != nil && errors.As(err, &re) {
		return re.rollback
	}
	return nil
}

// applyTarget atomically replaces the file at targetPath with newBytes, with
// rollback on a failed swap. The sequence (from minio/selfupdate CommitBinary):
//
//	write  <target>.new  (mode 0755)
//	remove <target>.old  (stale leftovers)
//	rename <target>     → <target>.old
//	rename <target>.new → <target>
//	on the final rename failure: rename <target>.old → <target> (rollback)
//	on success: remove <target>.old
//
// caller MUST gate on rawclaw's own verifyChecksum before calling this; applyTarget
// does no verification of its own. A *rollbackError return means the rollback also
// failed (manual reinstall needed); any other error means the original is intact.
func applyTarget(targetPath string, newBytes []byte) error {
	// Resolve symlinks so we replace the real binary, not a symlink pointing at it
	// (e.g. a Homebrew/install.sh shim in ~/bin → the actual file).
	if resolved, err := filepath.EvalSymlinks(targetPath); err == nil {
		targetPath = resolved
	}

	dir := filepath.Dir(targetPath)
	name := filepath.Base(targetPath)
	newPath := filepath.Join(dir, "."+name+".new")
	oldPath := filepath.Join(dir, "."+name+".old")

	// Write the replacement to a sibling temp file in the same directory (so the
	// final rename is atomic — same filesystem). 0755: it's an executable.
	if err := os.WriteFile(newPath, newBytes, 0o755); err != nil {
		return fmt.Errorf("write new binary: %w", err)
	}

	// Clear any stale .old, then move the current binary aside.
	_ = os.Remove(oldPath)
	if err := os.Rename(targetPath, oldPath); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("move current binary aside: %w", err)
	}

	// Swap the new binary into place. On failure, roll the original back.
	if err := os.Rename(newPath, targetPath); err != nil {
		if rbErr := os.Rename(oldPath, targetPath); rbErr != nil {
			// No binary where the executable used to be — manual recovery needed.
			return &rollbackError{commit: err, rollback: rbErr}
		}
		_ = os.Remove(newPath)
		return fmt.Errorf("swap in new binary (original restored): %w", err)
	}

	// Swap succeeded: drop the saved original.
	_ = os.Remove(oldPath)
	return nil
}
