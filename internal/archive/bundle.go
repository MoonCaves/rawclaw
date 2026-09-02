package archive

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ExportBundle packages the entire archive clone into a portable git bundle
// file at bundlePath. The clone must exist, be healthy, and have no unpushed
// commits before export.
func (a *Archive) ExportBundle(ctx context.Context, bundlePath string) error {
	if strings.TrimSpace(bundlePath) == "" {
		return errors.New("archive export bundle: bundle path required")
	}

	absBundle, err := filepath.Abs(bundlePath)
	if err != nil {
		return fmt.Errorf("archive export bundle: resolve bundle path: %w", err)
	}
	if st, err := os.Stat(absBundle); err == nil && st.IsDir() {
		return fmt.Errorf("archive export bundle: destination %q is a directory, expected a file path", bundlePath)
	}

	release, err := acquireSyncLock(ctx)
	if err != nil {
		return err
	}
	defer release()

	if err := a.ensureClone(ctx); err != nil {
		return fmt.Errorf("archive export bundle: %w", err)
	}

	if n, err := a.strandedCommits(ctx); err != nil {
		return fmt.Errorf("archive export bundle: check unpushed commits: %w", err)
	} else if n > 0 {
		return fmt.Errorf("archive export bundle: clone %s holds %d unpushed commit(s); push them first", a.clone, n)
	}

	if err := os.MkdirAll(filepath.Dir(absBundle), 0o755); err != nil {
		return fmt.Errorf("archive export bundle: create bundle dir: %w", err)
	}

	if _, err := a.run(ctx, a.clone, "bundle", "create", absBundle, "--all"); err != nil {
		outStr := err.Error()
		if strings.Contains(outStr, "Refusing to create empty bundle") || strings.Contains(outStr, "empty bundle") {
			return fmt.Errorf("archive clone has no commits to bundle; push transcripts first with 'rawclaw archive push'")
		}
		return fmt.Errorf("archive export bundle: create bundle: %w", err)
	}
	return nil
}

// InitFromBundle seeds a new archive from a pre-generated git bundle file,
// sets the upstream remote to remoteURL, stamps the completed clone sentinel,
// registers this machine, and persists the archive configuration.
func InitFromBundle(ctx context.Context, bundlePath, remoteURL string, machineName ...string) (*Archive, error) {
	if strings.TrimSpace(bundlePath) == "" {
		return nil, errors.New("archive init: bundle path required")
	}
	if strings.TrimSpace(remoteURL) == "" {
		return nil, errors.New("archive init: remote url required")
	}

	absBundle, err := filepath.Abs(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("archive init: resolve bundle path %q: %w", bundlePath, err)
	}
	if st, err := os.Stat(absBundle); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("archive init: bundle file %q not found", bundlePath)
		}
		return nil, fmt.Errorf("archive init: stat bundle file %q: %w", bundlePath, err)
	} else if st.IsDir() {
		return nil, fmt.Errorf("archive init: bundle path %q is a directory, expected a bundle file", bundlePath)
	}

	if _, err := readConfig(); err == nil {
		return nil, fmt.Errorf(
			"archive already initialized; remove %s (and %s) to re-initialize",
			configPath(), cloneDir())
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf(
			"archive config exists but is unreadable; remove %s to re-initialize: %w",
			configPath(), err)
	}

	name := defaultMachineName()
	if len(machineName) > 0 && machineName[0] != "" {
		name = machineName[0]
	}
	if err := validateMachineName(name); err != nil {
		return nil, fmt.Errorf("archive init: %w", err)
	}

	a := newArchive(Config{Remote: remoteURL, Name: name})

	release, err := acquireSyncLock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	// Double-checked locking: verify another process didn't initialize while we waited for the lock.
	if _, err := readConfig(); err == nil {
		return nil, fmt.Errorf(
			"archive already initialized; remove %s (and %s) to re-initialize",
			configPath(), cloneDir())
	}

	parent := filepath.Dir(a.clone)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("create archive state dir: %w", err)
	}

	// Verify bundle validity upfront before modifying any local clone state.
	if _, err := a.run(ctx, parent, "bundle", "verify", absBundle); err != nil {
		return nil, fmt.Errorf("archive init: verify bundle %s: %w", bundlePath, err)
	}

	if _, err := os.Stat(filepath.Join(a.clone, ".git")); err == nil {
		if n, serr := a.strandedCommits(ctx); serr != nil {
			return nil, fmt.Errorf("archive init: check unpushed commits in existing clone: %w", serr)
		} else if n > 0 {
			return nil, fmt.Errorf(
				"existing clone %s holds %d unpushed commit(s); push or back them up (git -C %s status), or delete the dir, then re-run init",
				a.clone, n, a.clone)
		}
	}
	if err := os.RemoveAll(a.clone); err != nil {
		return nil, fmt.Errorf("clear stale clone: %w", err)
	}

	if _, err := a.run(ctx, parent, "clone", absBundle, a.clone); err != nil {
		return nil, fmt.Errorf("clone from bundle %s: %w", bundlePath, err)
	}

	if _, err := a.run(ctx, a.clone, "remote", "set-url", "origin", remoteURL); err != nil {
		return nil, fmt.Errorf("set origin remote: %w", err)
	}

	gitDir := filepath.Join(a.clone, ".git")
	if err := os.WriteFile(filepath.Join(gitDir, cloneSentinel), nil, 0o644); err != nil {
		return nil, fmt.Errorf("stamp clone complete: %w", err)
	}

	if err := a.ensureRegistered(); err != nil {
		return nil, err
	}
	if err := a.ensureClone(ctx); err != nil {
		return nil, err
	}

	if err := writeConfig(a.cfg); err != nil {
		return nil, err
	}
	return a, nil
}
