package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Self-update is the bootstrapping feature: whatever release a user installs by
// hand is the LAST one they install by hand — every future version reaches them
// only if the running binary can replace itself. A bug here bricks the install
// base permanently (a broken updater can't ship its own fix), so the design is
// deliberately conservative: HTTPS only, mandatory sha256 verification of the
// downloaded asset against the release's checksums.txt (the security boundary),
// and an atomic replace-with-rollback (the standard POSIX swap, in apply.go)
// rather than a fragile in-place overwrite.

const (
	// upgradeRepo is the GitHub owner/repo releases are published under.
	upgradeRepo = "MoonCaves/rawclaw"

	// netTimeout bounds each network leg (API + asset + checksums) independently,
	// nested INSIDE the --timeout watchdog so the whole run still self-terminates.
	netTimeout = 60 * time.Second

	// exitUpdateAvailable is the distinct `--check` code meaning "an update exists"
	// (vs 0 = already current). Per golang-cli exit-code guidance: a scriptable,
	// non-error signal that is neither success-as-current nor a runtime failure.
	exitUpdateAvailable = 10
)

// release is the slice of the GitHub releases API we read: just the tag.
type release struct {
	TagName string `json:"tag_name"`
}

// newUpgradeCmd wires `rawclaw upgrade` (alias `update`): fetch the latest
// release, compare versions, and — if newer — download + checksum-verify +
// atomically replace the running binary.
func newUpgradeCmd(build BuildInfo) *cobra.Command {
	var (
		checkOnly bool
		force     bool
	)
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"update"},
		Short:   "Update rawclaw in place to the latest release",
		Long: "Download the latest rawclaw release from GitHub and replace this binary in place.\n\n" +
			"The download is verified against the release's sha256 checksums before it is applied; " +
			"a mismatch aborts without touching the installed binary. --check reports whether an " +
			"update is available without downloading anything.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpgrade(cmd, build, checkOnly, force)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&checkOnly, "check", false, "report whether an update is available without downloading (exit 10 if one exists)")
	f.BoolVar(&force, "force", false, "upgrade even from an unstamped dev build, or to a non-newer version")
	return cmd
}

// runUpgrade orchestrates the upgrade: resolve current version → fetch latest →
// compare → (check ? report : download+verify+apply). The HTTP client and the
// apply function are package-level vars so tests can drive the whole flow against
// an httptest server and a throwaway target file without hitting the network or
// the real executable.
func runUpgrade(cmd *cobra.Command, build BuildInfo, checkOnly, force bool) error {
	out := cmd.OutOrStdout()

	current := build.Version
	if current == "" {
		current = "dev"
	}
	isDev := !isStampedRelease(current)

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	latest, err := latestReleaseTag(ctx, upgradeHTTPClient, upgradeRepo)
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}

	cmpRes, err := compareVersions(current, latest)
	if err != nil && !isDev {
		// A dev build can't be compared meaningfully; only a stamped build that
		// fails to parse is a real problem.
		return fmt.Errorf("comparing versions: %w", err)
	}

	// "Newer release available" is true when the latest is strictly greater, OR
	// the running build is an unstamped dev (which we always treat as upgradeable).
	updateAvailable := isDev || cmpRes < 0

	if checkOnly {
		if updateAvailable {
			fmt.Fprintf(out, "update available: %s → %s\n", current, canonicalSemver(latest))
			return ExitError{Code: exitUpdateAvailable}
		}
		fmt.Fprintf(out, "rawclaw %s is already the latest\n", current)
		return nil
	}

	if !updateAvailable {
		fmt.Fprintf(out, "rawclaw %s is already the latest\n", current)
		return nil
	}

	if isDev && !force {
		return ExitError{
			Code: 2,
			Msg: fmt.Sprintf("this is an unstamped %q build; re-run with --force to replace it with %s",
				current, canonicalSemver(latest)),
		}
	}

	fmt.Fprintf(out, "downloading rawclaw %s for %s/%s ...\n", canonicalSemver(latest), runtime.GOOS, runtime.GOARCH)

	bin, err := downloadVerifiedBinary(ctx, upgradeHTTPClient, upgradeRepo, latest, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", latest, err)
	}

	if err := applyUpdate(bin); err != nil {
		// On a failed swap applyTarget tries to roll the original binary back into
		// place. asRollbackError is non-nil ONLY when that rollback ALSO failed — i.e.
		// the install may now be broken and needs manual recovery. Distinguish the
		// two so we never falsely claim the binary is intact.
		if rbErr := asRollbackError(err); rbErr != nil {
			return ExitError{
				Code: 1,
				Msg: fmt.Sprintf("upgrade FAILED and automatic rollback also failed: %v "+
					"(original binary may be missing — reinstall with install.sh)", rbErr),
			}
		}
		return fmt.Errorf("applying update (your existing binary is intact): %w", err)
	}

	fmt.Fprintf(out, "rawclaw %s → %s\n", current, canonicalSemver(latest))
	return nil
}

// isStampedRelease reports whether v looks like a real release tag (e.g. "v0.1.0"
// or "0.1.0") rather than the unstamped "dev" default.
func isStampedRelease(v string) bool {
	if v == "" || v == "dev" {
		return false
	}
	return isValidSemver(v)
}

// ensureV prefixes a bare version with "v" for display ("0.1.0" → "v0.1.0").
func ensureV(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// canonicalSemver normalises a version to a leading "v" for display. rawclaw only
// ever compares its own clean release tags, so this does not reformat the numbers.
func canonicalSemver(v string) string { return ensureV(v) }

// parseSemver parses "v?MAJOR.MINOR.PATCH(-prerelease)?(+build)?" into its numeric
// core and pre-release identifier. Build metadata is dropped (semver: not used for
// precedence). ok is false unless the core is exactly three non-negative integers.
func parseSemver(v string) (core [3]int, pre string, ok bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 { // drop build metadata
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 { // split off the pre-release
		pre = v[i+1:]
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return core, pre, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return core, pre, false
		}
		core[i] = n
	}
	return core, pre, true
}

// isValidSemver reports whether v is a MAJOR.MINOR.PATCH version (optional leading
// "v", optional pre-release/build).
func isValidSemver(v string) bool {
	_, _, ok := parseSemver(v)
	return ok
}

// compareSemver returns -1, 0, +1 for a vs b by semver precedence: numeric core
// first, then a release outranks an equal-core pre-release (1.0.0-rc < 1.0.0), and
// two pre-releases compare by identifier string (enough for our clean tags).
func compareSemver(a, b string) int {
	ca, pa, _ := parseSemver(a)
	cb, pb, _ := parseSemver(b)
	for i := 0; i < 3; i++ {
		if ca[i] != cb[i] {
			if ca[i] < cb[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case pa == pb:
		return 0
	case pa == "": // a is a release, b is a pre-release → a is newer
		return 1
	case pb == "":
		return -1
	case pa < pb:
		return -1
	default:
		return 1
	}
}

// compareVersions reports current vs latest as semver: -1 (current older),
// 0 (equal), +1 (current newer). An invalid version on either side is an error
// (the caller decides whether a dev current excuses it).
func compareVersions(current, latest string) (int, error) {
	if !isValidSemver(current) {
		return 0, fmt.Errorf("current version %q is not valid semver", current)
	}
	if !isValidSemver(latest) {
		return 0, fmt.Errorf("latest version %q is not valid semver", latest)
	}
	return compareSemver(current, latest), nil
}

// assetName returns the goreleaser archive name for a GOOS/GOARCH — mirroring
// install.sh and .goreleaser.yml: rawclaw_<os>_<arch>.tar.gz, except windows
// which ships a .zip.
func assetName(goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("rawclaw_%s_%s.%s", goos, goarch, ext)
}

// latestReleaseTag reads the latest release tag via the GitHub API, falling back
// to the releases/latest redirect (the Location URL ends in the tag). HTTPS only.
func latestReleaseTag(ctx context.Context, client *http.Client, repo string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "rawclaw-self-update")

	// apiErr captures why the JSON API path failed (transport error or a non-200
	// status) so that, if the redirect fallback also fails, we can surface the real
	// cause — a GitHub 403 rate-limit, a 404, or a 5xx — instead of a vague failure.
	var apiErr error
	resp, err := client.Do(req)
	if err != nil {
		apiErr = err
	} else {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var rel release
			if derr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); derr == nil && rel.TagName != "" {
				return rel.TagName, nil
			}
			apiErr = fmt.Errorf("github api %s returned 200 but no usable tag_name", apiURL)
		} else {
			apiErr = fmt.Errorf("github api %s: status %d", apiURL, resp.StatusCode)
		}
		// Drain so the connection can be reused before we try the fallback.
		_, _ = io.Copy(io.Discard, resp.Body)
	}

	tag, ferr := latestTagViaRedirect(ctx, client, repo)
	if ferr != nil {
		return "", fmt.Errorf("github api failed (%v) and redirect fallback failed: %w", apiErr, ferr)
	}
	return tag, nil
}

// latestTagViaRedirect resolves the tag from the releases/latest HTML redirect:
// GitHub 302s to .../releases/tag/<tag>. We follow no redirects and read the
// Location header so a single request yields the tag.
func latestTagViaRedirect(ctx context.Context, client *http.Client, repo string) (string, error) {
	url := fmt.Sprintf("https://github.com/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "rawclaw-self-update")

	// A client that refuses to follow redirects, so resp is the 3xx itself and we
	// can read Location. We copy the caller's Transport so test servers are honoured.
	noFollow := &http.Client{
		Transport: client.Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noFollow.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", fmt.Errorf("no Location header on releases/latest (status %d)", resp.StatusCode)
	}
	if i := strings.LastIndex(loc, "/tag/"); i >= 0 {
		tag := loc[i+len("/tag/"):]
		if tag != "" {
			return tag, nil
		}
	}
	return "", fmt.Errorf("could not parse tag from redirect %q", loc)
}

// downloadVerifiedBinary downloads the platform archive AND checksums.txt for a
// release, verifies the archive's sha256 against its checksums.txt line (REFUSING
// on any mismatch or missing entry — this is the security boundary), then extracts
// and returns the rawclaw binary bytes.
func downloadVerifiedBinary(ctx context.Context, client *http.Client, repo, tag, goos, goarch string) ([]byte, error) {
	asset := assetName(goos, goarch)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, tag)

	archive, err := httpGetBytes(ctx, client, base+"/"+asset)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", asset, err)
	}
	sums, err := httpGetBytes(ctx, client, base+"/checksums.txt")
	if err != nil {
		return nil, fmt.Errorf("fetch checksums.txt: %w", err)
	}

	if err := verifyChecksum(asset, archive, sums); err != nil {
		return nil, err
	}

	bin, err := extractBinary(goos, archive)
	if err != nil {
		return nil, fmt.Errorf("extract %s from %s: %w", binaryName(goos), asset, err)
	}
	return bin, nil
}

// verifyChecksum REFUSES the download unless the sha256 of data matches the line
// for asset in a goreleaser checksums.txt ("<hex>  <filename>"). A missing line or
// any mismatch is a hard error: a wrong or partial download must NEVER be applied.
func verifyChecksum(asset string, data, checksums []byte) error {
	want, ok := checksumFor(asset, checksums)
	if !ok {
		return fmt.Errorf("no checksum for %s in checksums.txt — refusing to apply unverified download", asset)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s — refusing to apply", asset, want, got)
	}
	return nil
}

// checksumFor finds the lowercased hex sha256 for asset in a checksums.txt body.
// Each line is "<hex>  <filename>" (goreleaser uses two spaces; we tolerate any
// run of whitespace and an optional leading "*" binary-mode marker).
func checksumFor(asset string, checksums []byte) (string, bool) {
	for _, line := range strings.Split(string(checksums), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == asset {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

// binaryName is the in-archive executable name for a GOOS (windows adds .exe).
func binaryName(goos string) string {
	if goos == "windows" {
		return "rawclaw.exe"
	}
	return "rawclaw"
}

// extractBinary pulls the rawclaw executable out of the release archive in
// memory: tar.gz for unix, zip for windows.
func extractBinary(goos string, archive []byte) ([]byte, error) {
	if goos == "windows" {
		return extractFromZip(archive, binaryName(goos))
	}
	return extractFromTarGz(archive, binaryName(goos))
}

// extractFromTarGz returns the named file's bytes from a gzip-compressed tarball.
func extractFromTarGz(archive []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// goreleaser places the binary at the archive root; match by basename so a
		// nested layout still resolves.
		if baseName(hdr.Name) == name {
			// Bound the copy: a release binary is a handful of MB; cap well above that.
			return io.ReadAll(io.LimitReader(tr, 512<<20))
		}
	}
	return nil, fmt.Errorf("%q not found in archive", name)
}

// extractFromZip returns the named file's bytes from a zip archive (windows).
func extractFromZip(archive []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	for _, f := range zr.File {
		if baseName(f.Name) == name {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open %s in zip: %w", name, err)
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, 512<<20))
		}
	}
	return nil, fmt.Errorf("%q not found in zip", name)
}

// httpGetBytes does a bounded HTTPS GET and returns the body. Each call gets its
// own context deadline so a stalled connection can't outlast the watchdog.
func httpGetBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "rawclaw-self-update")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 512<<20))
}

// upgradeHTTPClient is the client used for all self-update requests. It's a
// package var so tests can point it at an httptest server (and to avoid the
// default client's unbounded behaviour). The per-request context deadlines are
// the real timeout; this is a generous backstop.
var upgradeHTTPClient = &http.Client{Timeout: 5 * time.Minute}

// applyUpdate is the atomic binary-replace primitive: resolve the running
// executable's real path (os.Executable → EvalSymlinks via applyTarget), then write
// a sibling .new, move current → .old, swap .new → current, rolling .old back on a
// failed swap (see apply.go). It's a package var so the integration test can swap in
// a version that targets a throwaway file instead of the running executable.
var applyUpdate = func(bin []byte) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate running executable: %w", err)
	}
	return applyTarget(exe, bin)
}
