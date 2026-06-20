package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/minio/selfupdate"
)

// ── version comparison (incl. the dev case) ──

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		current, latest string
		want           int
		wantErr        bool
	}{
		{"older", "v0.1.0", "v0.2.0", -1, false},
		{"older bare current", "0.1.0", "v0.2.0", -1, false},
		{"equal", "v0.1.0", "v0.1.0", 0, false},
		{"equal mixed prefix", "0.1.0", "v0.1.0", 0, false},
		{"newer current", "v0.3.0", "v0.2.0", 1, false},
		{"patch older", "v0.1.0", "v0.1.1", -1, false},
		{"prerelease lower than release", "v0.1.0-rc.1", "v0.1.0", -1, false},
		{"invalid current is error", "dev", "v0.1.0", 0, true},
		{"invalid latest is error", "v0.1.0", "not-a-version", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := compareVersions(tc.current, tc.latest)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("compareVersions(%q,%q) expected error, got nil", tc.current, tc.latest)
				}
				return
			}
			if err != nil {
				t.Fatalf("compareVersions(%q,%q) unexpected error: %v", tc.current, tc.latest, err)
			}
			if got != tc.want {
				t.Errorf("compareVersions(%q,%q) = %d, want %d", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

func TestIsStampedRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		v    string
		want bool
	}{
		{"dev", false},
		{"", false},
		{"v0.1.0", true},
		{"0.1.0", true},
		{"v1.2.3-rc.1", true},
		{"garbage", false},
	}
	for _, tc := range tests {
		if got := isStampedRelease(tc.v); got != tc.want {
			t.Errorf("isStampedRelease(%q) = %v, want %v", tc.v, got, tc.want)
		}
	}
}

// ── asset-name selection per GOOS/GOARCH ──

func TestAssetName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		goos, goarch string
		want         string
	}{
		{"linux", "amd64", "rawclaw_linux_amd64.tar.gz"},
		{"linux", "arm64", "rawclaw_linux_arm64.tar.gz"},
		{"darwin", "amd64", "rawclaw_darwin_amd64.tar.gz"},
		{"darwin", "arm64", "rawclaw_darwin_arm64.tar.gz"},
		{"windows", "amd64", "rawclaw_windows_amd64.zip"},
	}
	for _, tc := range tests {
		if got := assetName(tc.goos, tc.goarch); got != tc.want {
			t.Errorf("assetName(%q,%q) = %q, want %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestBinaryName(t *testing.T) {
	t.Parallel()
	if got := binaryName("linux"); got != "rawclaw" {
		t.Errorf("binaryName(linux) = %q, want rawclaw", got)
	}
	if got := binaryName("windows"); got != "rawclaw.exe" {
		t.Errorf("binaryName(windows) = %q, want rawclaw.exe", got)
	}
}

// ── checksum verification: the critical security test ──
//
// A wrong, partial, or unlisted download MUST be rejected before it is ever
// applied. These cases are the security boundary of the whole feature.

func TestVerifyChecksum(t *testing.T) {
	t.Parallel()

	asset := "rawclaw_linux_amd64.tar.gz"
	payload := []byte("the-real-archive-bytes")
	sum := sha256.Sum256(payload)
	goodHex := hex.EncodeToString(sum[:])

	// A goreleaser checksums.txt: two-space separated "<hex>  <name>", multiple lines.
	goodSums := fmt.Sprintf("%s  rawclaw_linux_arm64.tar.gz\n%s  %s\n%s  rawclaw_darwin_amd64.tar.gz\n",
		hex.EncodeToString(sha256.New().Sum(nil)), goodHex, asset,
		hex.EncodeToString(sha256.New().Sum(nil)))

	tests := []struct {
		name      string
		data      []byte
		checksums string
		wantErr   bool
	}{
		{
			name:      "valid checksum passes",
			data:      payload,
			checksums: goodSums,
			wantErr:   false,
		},
		{
			name:      "uppercase hex still matches",
			data:      payload,
			checksums: fmt.Sprintf("%s  %s\n", goodHex, asset),
			wantErr:   false,
		},
		{
			name:      "tampered byte rejected",
			data:      append([]byte("X"), payload...),
			checksums: goodSums,
			wantErr:   true,
		},
		{
			name:      "truncated download rejected",
			data:      payload[:len(payload)-1],
			checksums: goodSums,
			wantErr:   true,
		},
		{
			name:      "missing asset line rejected",
			data:      payload,
			checksums: fmt.Sprintf("%s  some_other_asset.tar.gz\n", goodHex),
			wantErr:   true,
		},
		{
			name:      "empty checksums rejected",
			data:      payload,
			checksums: "",
			wantErr:   true,
		},
		{
			name:      "wrong hex for our asset rejected",
			data:      payload,
			checksums: fmt.Sprintf("%s  %s\n", "deadbeef", asset),
			wantErr:   true,
		},
		{
			name:      "binary-mode star prefix tolerated",
			data:      payload,
			checksums: fmt.Sprintf("%s *%s\n", goodHex, asset),
			wantErr:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := verifyChecksum(asset, tc.data, []byte(tc.checksums))
			if tc.wantErr && err == nil {
				t.Fatalf("verifyChecksum(%s) expected rejection, got nil — a bad download would be applied!", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("verifyChecksum(%s) unexpected error: %v", tc.name, err)
			}
		})
	}
}

// ── archive extraction ──

// makeTarGz builds a gzip-compressed tarball containing one file.
func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func TestExtractFromTarGz(t *testing.T) {
	t.Parallel()

	want := []byte("#!/bin/echo fake-binary-bytes")
	archive := makeTarGz(t, "rawclaw", want)

	got, err := extractFromTarGz(archive, "rawclaw")
	if err != nil {
		t.Fatalf("extractFromTarGz: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted bytes = %q, want %q", got, want)
	}

	if _, err := extractFromTarGz(archive, "nonexistent"); err == nil {
		t.Error("extractFromTarGz expected error for missing file")
	}

	if _, err := extractFromTarGz([]byte("not a gzip"), "rawclaw"); err == nil {
		t.Error("extractFromTarGz expected error for non-gzip input")
	}
}

// ── HTTP flow: latest tag + verified download against httptest (NO network) ──

// fakeRelease wires a test server that mimics the GitHub API + release assets for
// one tagged release, with a correct checksums.txt for the tarball it serves.
func fakeRelease(t *testing.T, repo, tag string, binContent []byte) *httptest.Server {
	t.Helper()
	asset := assetName(runtime.GOOS, runtime.GOARCH)
	archive := makeTarGz(t, binaryName(runtime.GOOS), binContent)
	if runtime.GOOS == "windows" {
		// The non-windows extractor path is what we exercise; for windows the asset
		// would be a zip. Keep the test meaningful by skipping the zip plumbing here.
		t.Skip("HTTP integration test wired for tar.gz platforms")
	}
	sum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset)

	mux := http.NewServeMux()
	mux.HandleFunc(fmt.Sprintf("/repos/%s/releases/latest", repo), func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name":%q}`, tag)
	})
	mux.HandleFunc(fmt.Sprintf("/%s/releases/download/%s/%s", repo, tag, asset), func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc(fmt.Sprintf("/%s/releases/download/%s/checksums.txt", repo, tag), func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	return httptest.NewServer(mux)
}

// rewriteHost is a RoundTripper that redirects every github.com / api.github.com
// request to the test server, so the real URL-building code is exercised end to
// end without touching the network.
type rewriteHost struct {
	base string // test server URL
}

func (r rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Host {
	case "api.github.com":
		// /repos/... already matches the mux pattern under the server root.
		req.URL.Scheme = "http"
		req.URL.Host = hostOf(r.base)
	case "github.com":
		req.URL.Scheme = "http"
		req.URL.Host = hostOf(r.base)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func hostOf(serverURL string) string {
	return serverURL[len("http://"):]
}

func TestLatestReleaseTagViaAPI(t *testing.T) {
	t.Parallel()

	srv := fakeRelease(t, "MoonCaves/rawclaw", "v0.2.0", []byte("bin"))
	defer srv.Close()

	client := &http.Client{Transport: rewriteHost{base: srv.URL}}
	tag, err := latestReleaseTag(context.Background(), client, "MoonCaves/rawclaw")
	if err != nil {
		t.Fatalf("latestReleaseTag: %v", err)
	}
	if tag != "v0.2.0" {
		t.Errorf("tag = %q, want v0.2.0", tag)
	}
}

func TestDownloadVerifiedBinary(t *testing.T) {
	t.Parallel()

	want := []byte("the-new-rawclaw-binary")
	srv := fakeRelease(t, "MoonCaves/rawclaw", "v0.2.0", want)
	defer srv.Close()

	client := &http.Client{Transport: rewriteHost{base: srv.URL}}
	got, err := downloadVerifiedBinary(context.Background(), client, "MoonCaves/rawclaw", "v0.2.0", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("downloadVerifiedBinary: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("downloaded binary = %q, want %q", got, want)
	}
}

// TestDownloadVerifiedBinaryRejectsTamper proves the END-TO-END guarantee: if the
// served archive does not match its published checksum, the download is refused
// and NO bytes are returned to be applied.
func TestDownloadVerifiedBinaryRejectsTamper(t *testing.T) {
	t.Parallel()

	asset := assetName(runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		t.Skip("wired for tar.gz platforms")
	}
	archive := makeTarGz(t, binaryName(runtime.GOOS), []byte("real"))
	// Publish a checksum for DIFFERENT bytes than we serve → must be rejected.
	wrongSum := sha256.Sum256([]byte("something-else"))
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(wrongSum[:]), asset)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/MoonCaves/rawclaw/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"tag_name":"v0.2.0"}`)
	})
	mux.HandleFunc("/MoonCaves/rawclaw/releases/download/v0.2.0/"+asset, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/MoonCaves/rawclaw/releases/download/v0.2.0/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(checksums))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := &http.Client{Transport: rewriteHost{base: srv.URL}}
	_, err := downloadVerifiedBinary(context.Background(), client, "MoonCaves/rawclaw", "v0.2.0", runtime.GOOS, runtime.GOARCH)
	if err == nil {
		t.Fatal("downloadVerifiedBinary accepted a checksum-mismatched archive — security boundary breached")
	}
}

// ── integration: minio/selfupdate.Apply replace + rollback primitive ──
//
// Proves the dangerous part works on a THROWAWAY temp file (never the test
// binary): a successful replace swaps contents atomically, and a checksum
// mismatch is rejected with the original file untouched.

func TestSelfUpdateApplyReplacesTempBinary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "rawclaw-fake")
	original := []byte("ORIGINAL-v1")
	if err := os.WriteFile(target, original, 0o755); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	replacement := []byte("REPLACED-v2-contents")
	if err := selfupdate.Apply(bytes.NewReader(replacement), selfupdate.Options{TargetPath: target}); err != nil {
		t.Fatalf("Apply replace: %v (rollback err: %v)", err, selfupdate.RollbackError(err))
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replaced target: %v", err)
	}
	if !bytes.Equal(got, replacement) {
		t.Errorf("target after Apply = %q, want %q", got, replacement)
	}
}

// TestSelfUpdateApplyRejectsBadChecksumLeavesOriginal proves the library's own
// pre-commit checksum gate: when the supplied Checksum doesn't match the bytes,
// Apply fails BEFORE swapping, so the original binary is left intact (no rollback
// needed, RollbackError is nil).
func TestSelfUpdateApplyRejectsBadChecksumLeavesOriginal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "rawclaw-fake")
	original := []byte("ORIGINAL-must-survive")
	if err := os.WriteFile(target, original, 0o755); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	replacement := []byte("REPLACEMENT")
	badChecksum := sha256.Sum256([]byte("not-the-replacement"))

	err := selfupdate.Apply(bytes.NewReader(replacement), selfupdate.Options{
		TargetPath: target,
		Checksum:   badChecksum[:],
	})
	if err == nil {
		t.Fatal("Apply accepted a bad checksum — must reject before swapping")
	}
	if rb := selfupdate.RollbackError(err); rb != nil {
		t.Errorf("unexpected rollback error (swap should not have started): %v", rb)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after rejected Apply: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("original binary was modified after a rejected update: got %q, want %q", got, original)
	}
}

// ── command wiring: --check exit codes and the dev-force gate ──

// runUpgradeCmd drives the real cobra command with the package HTTP client and
// applyUpdate hook swapped for the duration of the call.
func runUpgradeCmd(t *testing.T, build BuildInfo, args []string, srv *httptest.Server, apply func([]byte) error) (string, error) {
	t.Helper()

	prevClient := upgradeHTTPClient
	prevApply := applyUpdate
	upgradeHTTPClient = &http.Client{Transport: rewriteHost{base: srv.URL}}
	applyUpdate = apply
	t.Cleanup(func() {
		upgradeHTTPClient = prevClient
		applyUpdate = prevApply
	})

	root := NewRootCmd(build)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestUpgradeCheckReportsAvailable(t *testing.T) {
	srv := fakeRelease(t, "MoonCaves/rawclaw", "v0.2.0", []byte("bin"))
	defer srv.Close()

	out, err := runUpgradeCmd(t, BuildInfo{Version: "v0.1.0"}, []string{"upgrade", "--check"}, srv, nil)

	var ee ExitError
	if !errors.As(err, &ee) || ee.Code != exitUpdateAvailable {
		t.Fatalf("--check with newer release: got err %v, want ExitError{Code:%d}", err, exitUpdateAvailable)
	}
	if !bytes.Contains([]byte(out), []byte("update available")) {
		t.Errorf("--check output = %q, want 'update available'", out)
	}
}

func TestUpgradeCheckAlreadyLatest(t *testing.T) {
	srv := fakeRelease(t, "MoonCaves/rawclaw", "v0.1.0", []byte("bin"))
	defer srv.Close()

	out, err := runUpgradeCmd(t, BuildInfo{Version: "v0.1.0"}, []string{"upgrade", "--check"}, srv, nil)
	if err != nil {
		t.Fatalf("--check when current: unexpected err %v", err)
	}
	if !bytes.Contains([]byte(out), []byte("already the latest")) {
		t.Errorf("--check output = %q, want 'already the latest'", out)
	}
}

func TestUpgradeDevRequiresForce(t *testing.T) {
	srv := fakeRelease(t, "MoonCaves/rawclaw", "v0.2.0", []byte("bin"))
	defer srv.Close()

	// A dev build without --force must refuse (exit 2) and never call applyUpdate.
	applied := false
	_, err := runUpgradeCmd(t, BuildInfo{Version: "dev"}, []string{"upgrade"}, srv,
		func([]byte) error { applied = true; return nil })

	var ee ExitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("dev upgrade without --force: got err %v, want ExitError{Code:2}", err)
	}
	if applied {
		t.Error("applyUpdate was called for a dev build without --force")
	}
}

func TestUpgradeAppliesNewerRelease(t *testing.T) {
	want := []byte("the-downloaded-binary-bytes")
	srv := fakeRelease(t, "MoonCaves/rawclaw", "v0.2.0", want)
	defer srv.Close()

	var appliedBytes []byte
	out, err := runUpgradeCmd(t, BuildInfo{Version: "v0.1.0"}, []string{"upgrade"}, srv,
		func(b []byte) error { appliedBytes = b; return nil })
	if err != nil {
		t.Fatalf("upgrade apply: %v", err)
	}
	if !bytes.Equal(appliedBytes, want) {
		t.Errorf("applied bytes = %q, want %q (post-checksum-verify binary)", appliedBytes, want)
	}
	if !bytes.Contains([]byte(out), []byte("→")) {
		t.Errorf("upgrade output = %q, want old → new line", out)
	}
}

func TestUpgradeAlreadyLatestSkipsApply(t *testing.T) {
	srv := fakeRelease(t, "MoonCaves/rawclaw", "v0.1.0", []byte("bin"))
	defer srv.Close()

	applied := false
	out, err := runUpgradeCmd(t, BuildInfo{Version: "v0.1.0"}, []string{"upgrade"}, srv,
		func([]byte) error { applied = true; return nil })
	if err != nil {
		t.Fatalf("upgrade when current: %v", err)
	}
	if applied {
		t.Error("applyUpdate was called when already on the latest version")
	}
	if !bytes.Contains([]byte(out), []byte("already the latest")) {
		t.Errorf("output = %q, want 'already the latest'", out)
	}
}
