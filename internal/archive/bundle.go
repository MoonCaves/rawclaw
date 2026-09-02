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

	release, err := acquireSyncLock(ctx)
	if err != nil {
		return err
	}
	defer release()

	gitDir := filepath.Join(a.clone, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("archive export bundle: clone %s does not exist", a.clone)
		}
		return fmt.Errorf("archive export bundle: stat clone git dir: %w", err)
	}

	if err := a.ensureClone(ctx); err != nil {
		return fmt.Errorf("archive export bundle: %w", err)
	}

	if ahead, err := a.aheadOfRemote(ctx); err != nil {
		return fmt.Errorf("archive export bundle: check unpushed commits: %w", err)
	} else if ahead {
		return fmt.Errorf("archive export bundle: clone %s holds unpushed commits; push them first", a.clone)
	}

	if n, err := a.strandedCommits(ctx); err == nil && n > 0 {
		return fmt.Errorf("archive export bundle: clone %s holds %d unpushed commit(s); push them first", a.clone, n)
	}

	absBundle, err := filepath.Abs(bundlePath)
	if err != nil {
		return fmt.Errorf("archive export bundle: resolve bundle path: %w", err)
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
func InitFromBundle(ctx context.Context, bundlePath, remoteURL string, machineName ...string) error {
	if strings.TrimSpace(bundlePath) == "" {
		return errors.New("archive init: bundle path required")
	}
	if strings.TrimSpace(remoteURL) == "" {
		return errors.New("archive init: remote url required")
	}

	absBundle, err := filepath.Abs(bundlePath)
	if err != nil {
		return fmt.Errorf("archive init: resolve bundle path %q: %w", bundlePath, err)
	}
	if st, err := os.Stat(absBundle); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("archive init: bundle file %q not found", bundlePath)
		}
		return fmt.Errorf("archive init: stat bundle file %q: %w", bundlePath, err)
	} else if st.IsDir() {
		return fmt.Errorf("archive init: bundle path %q is a directory, expected a bundle file", bundlePath)
	}

	if _, err := readConfig(); err == nil {
		return fmt.Errorf(
			"archive already initialized; remove %s (and %s) to re-initialize",
			configPath(), cloneDir())
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf(
			"archive config exists but is unreadable; remove %s to re-initialize: %w",
			configPath(), err)
	}

	name := defaultMachineName()
	if len(machineName) > 0 && machineName[0] != "" {
		name = machineName[0]
	}
	if err := validateMachineName(name); err != nil {
		return fmt.Errorf("archive init: %w", err)
	}

	a := newArchive(Config{Remote: remoteURL, Name: name})

	release, err := acquireSyncLock(ctx)
	if err != nil {
		return err
	}
	defer release()

	if _, err := os.Stat(filepath.Join(a.clone, ".git")); err == nil {
		if n, serr := a.strandedCommits(ctx); serr == nil && n > 0 {
			return fmt.Errorf(
				"existing clone %s holds %d unpushed commit(s); push or back them up (git -C %s status), or delete the dir, then re-run init",
				a.clone, n, a.clone)
		}
	}
	if err := os.RemoveAll(a.clone); err != nil {
		return fmt.Errorf("clear stale clone: %w", err)
	}

	parent := filepath.Dir(a.clone)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create archive state dir: %w", err)
	}

	if _, err := a.run(ctx, parent, "clone", absBundle, a.clone); err != nil {
		return fmt.Errorf("clone from bundle %s: %w", bundlePath, err)
	}

	if _, err := a.run(ctx, a.clone, "remote", "set-url", "origin", remoteURL); err != nil {
		return fmt.Errorf("set origin remote: %w", err)
	}

	gitDir := filepath.Join(a.clone, ".git")
	if err := os.WriteFile(filepath.Join(gitDir, cloneSentinel), nil, 0o644); err != nil {
		return fmt.Errorf("stamp clone complete: %w", err)
	}

	if err := a.ensureRegistered(); err != nil {
		return err
	}
	if err := a.ensureClone(ctx); err != nil {
		return err
	}

	return writeConfig(a.cfg)
}
