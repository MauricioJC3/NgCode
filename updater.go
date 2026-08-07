package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// updater.go holds all auto-update process/network/filesystem logic behind
// small unexported functions, mirroring lsp.go's file-per-concern split
// (lsp.go handles the gopls subprocess; this file handles the release
// download/verify/swap flow). app.go, in a later PR, exposes a thin
// updateState field plus a couple of bound Update* methods that only
// orchestrate the functions below — no process/network/filesystem logic
// lives in app.go itself.
//
// GitHub Releases contract this file consumes (owner/repo fixed, no auth):
//
//	GET https://api.github.com/repos/MauricioJC3/NgCode/releases/latest
//
// Asset naming contract this file produces via assetNameForPlatform() and
// MUST match byte-for-byte, produced by .github/workflows/release.yml's
// "Rename binary/installer to release asset name" steps (verified against
// that file on the feat/auto-update-ci-config branch, commit 0ea40d8):
//
//	ngcode-windows-amd64.exe            (raw binary, windows/amd64)
//	ngcode-darwin-amd64                 (raw binary, darwin/amd64)
//	ngcode-darwin-arm64                 (raw binary, darwin/arm64)
//	ngcode-windows-amd64-installer.exe  (NSIS installer, not consumed by
//	                                      the in-app updater — download-only
//	                                      users get this from the Release
//	                                      page directly)
//	checksums.txt                       (sha256sum/shasum -a 256 format,
//	                                      one "<hex>  <filename>" line per
//	                                      asset above)
//
// Note: release.yml only builds windows/amd64, darwin/amd64 and
// darwin/arm64 — there is deliberately no linux or windows/arm64 asset, so
// assetNameForPlatform() returns an explicit error for any other
// GOOS/GOARCH combination rather than guessing a name that CI never
// produces.

// updateState describes a pending, already-verified-as-newer release ready
// to be offered to the user. Populated by checkForUpdatesBackground and
// consumed by app.go's UpdateAccept in a later PR.
type updateState struct {
	Version     string
	AssetURL    string
	ChecksumURL string
}

// UpdateInfo is the event-payload / bound-method-return shape the frontend
// receives — deliberately smaller than updateState (no URLs), since the
// frontend never needs the raw download URLs, only what to show the user.
type UpdateInfo struct {
	Version        string `json:"version"`
	CurrentVersion string `json:"currentVersion"`
}

// ---- semver ----

// parseSemver parses a version string of the form "vMAJOR.MINOR.PATCH" or
// "MAJOR.MINOR.PATCH" (the leading "v" is optional) into its integer
// components. No prerelease/build-metadata support: release tags produced
// by release.yml are always plain semver, so a hand-rolled parser avoids
// pulling in a semver library for logic this simple (matches the existing
// project convention of avoiding dependencies for simple parsing, see
// toFileURI/fromFileURI in lsp.go).
func parseSemver(s string) (major, minor, patch int, err error) {
	trimmed := strings.TrimPrefix(s, "v")
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid semver %q: expected MAJOR.MINOR.PATCH", s)
	}

	nums := make([]int, 3)
	for i, p := range parts {
		n, convErr := strconv.Atoi(p)
		if convErr != nil {
			return 0, 0, 0, fmt.Errorf("invalid semver %q: %w", s, convErr)
		}
		nums[i] = n
	}

	return nums[0], nums[1], nums[2], nil
}

// compareSemver returns -1 if a<b, 0 if a==b, 1 if a>b. A malformed input
// sorts as lower than any valid version, and two malformed inputs compare
// equal — this is a defensive default so a malformed remote response can
// never be mistaken for "newer" than the running version.
func compareSemver(a, b string) int {
	aMajor, aMinor, aPatch, aErr := parseSemver(a)
	bMajor, bMinor, bPatch, bErr := parseSemver(b)

	switch {
	case aErr != nil && bErr != nil:
		return 0
	case aErr != nil:
		return -1
	case bErr != nil:
		return 1
	}

	if aMajor != bMajor {
		return cmpInt(aMajor, bMajor)
	}
	if aMinor != bMinor {
		return cmpInt(aMinor, bMinor)
	}
	return cmpInt(aPatch, bPatch)
}

func cmpInt(x, y int) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	default:
		return 0
	}
}

// ---- asset naming ----

// goos/goarch are seams over runtime.GOOS/runtime.GOARCH so tests can
// exercise every platform branch of assetNameForPlatform without needing
// to actually cross-compile the test binary for each target.
var (
	goos   = runtime.GOOS
	goarch = runtime.GOARCH
)

// assetNameForPlatform returns the release asset filename this binary
// should download for the current platform, matching
// .github/workflows/release.yml's naming contract byte-for-byte. Only the
// platforms release.yml actually builds are supported; anything else is an
// explicit error rather than a guessed/absent name.
func assetNameForPlatform() (string, error) {
	switch {
	case goos == "windows" && goarch == "amd64":
		return "ngcode-windows-amd64.exe", nil
	case goos == "darwin" && goarch == "amd64":
		return "ngcode-darwin-amd64", nil
	case goos == "darwin" && goarch == "arm64":
		return "ngcode-darwin-arm64", nil
	default:
		return "", fmt.Errorf("auto-update is not supported on %s/%s", goos, goarch)
	}
}

// ---- update-check cache ----

const updateCheckInterval = 24 * time.Hour

// updateCache is the persisted shape of update-check.json.
type updateCache struct {
	LastCheck           time.Time `json:"lastCheck"`
	LastNotifiedVersion string    `json:"lastNotifiedVersion"`
}

// updateCacheFilePath returns the path to the persisted update-check cache
// file: $UserConfigDir/ngcode/update-check.json. This is the exact,
// documented on-disk location — os.UserConfigDir() is the idiomatic Go
// cross-platform location (%AppData% on Windows, ~/Library/Application
// Support on macOS, $XDG_CONFIG_HOME or ~/.config on Linux) and needs no
// new dependency.
func updateCacheFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ngcode", "update-check.json"), nil
}

// loadUpdateCache reads the cache file. A missing file or corrupt JSON is
// deliberately NOT an error: both are treated as "never checked before"
// (zero-value cache), since a first run or a hand-edited/truncated cache
// file should never block the update flow.
func loadUpdateCache() (updateCache, error) {
	path, err := updateCacheFilePath()
	if err != nil {
		return updateCache{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return updateCache{}, nil
		}
		return updateCache{}, err
	}

	var cache updateCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return updateCache{}, nil // corrupt cache: treat as never-checked
	}
	return cache, nil
}

// saveUpdateCache persists cache to disk, creating the ngcode config
// directory if it doesn't exist yet.
func saveUpdateCache(cache updateCache) error {
	path, err := updateCacheFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// shouldCheckForUpdates reports whether enough time (updateCheckInterval)
// has passed since the last recorded check to justify a new network call.
func shouldCheckForUpdates(cache updateCache, now time.Time) bool {
	if cache.LastCheck.IsZero() {
		return true
	}
	return now.Sub(cache.LastCheck) >= updateCheckInterval
}

// ---- checksum verification ----

// verifyChecksum computes the SHA256 of the file at path and compares it,
// case-insensitively, against wantHex. It never mutates or removes path —
// callers own cleanup on mismatch (see checkForUpdatesBackground's callers
// in app.go, PR3, which discard the temp file on any pre-swap failure).
func verifyChecksum(path, wantHex string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, wantHex) {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", path, got, wantHex)
	}
	return nil
}

// ---- GitHub Releases API client ----

const updateCheckHTTPTimeout = 10 * time.Second

// githubReleasesURL is a seam so tests can point checkForUpdatesBackground
// at an httptest.Server instead of the real GitHub API.
var githubReleasesURL = "https://api.github.com/repos/MauricioJC3/NgCode/releases/latest"

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// fetchLatestRelease performs the unauthenticated GET against GitHub's
// releases/latest API with a bounded timeout. The public repo's
// unauthenticated rate limit (60 requests/hour per IP) is far above a
// once-per-24h check, so no token is ever sent.
func fetchLatestRelease(ctx context.Context) (*githubRelease, error) {
	ctx, cancel := context.WithTimeout(ctx, updateCheckHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

// checkForUpdatesBackground checks, at most once per updateCheckInterval,
// whether a newer release than currentVersion is published on GitHub.
//
// Contract: a non-nil error means the check itself failed (offline,
// rate-limited, malformed response, or a local cache I/O problem) — the
// cache's lastCheck is deliberately NOT updated in that case, so the next
// launch retries instead of waiting out the full 24h window (per spec
// "No connectivity or rate limit exhausted"). The caller (app.go's
// startup(), PR3) is responsible for logging a non-nil error; this
// function only decides whether to retry, not how to report to the user.
// A nil error with a nil *updateState means the check succeeded but there
// is nothing new to offer (already latest, cache window not elapsed yet,
// or the release has no matching asset for this platform).
func checkForUpdatesBackground(ctx context.Context, currentVersion string) (*updateState, error) {
	cache, err := loadUpdateCache()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if !shouldCheckForUpdates(cache, now) {
		return nil, nil
	}

	release, err := fetchLatestRelease(ctx)
	if err != nil {
		return nil, err
	}

	cache.LastCheck = now

	if compareSemver(release.TagName, currentVersion) <= 0 {
		return nil, saveUpdateCache(cache)
	}

	assetName, err := assetNameForPlatform()
	if err != nil {
		// Unsupported platform: the check itself succeeded, there's just
		// nothing this binary could ever swap itself for. Persist the
		// cache so we don't hammer the API every launch.
		return nil, saveUpdateCache(cache)
	}

	var assetURL, checksumURL string
	for _, a := range release.Assets {
		switch a.Name {
		case assetName:
			assetURL = a.BrowserDownloadURL
		case "checksums.txt":
			checksumURL = a.BrowserDownloadURL
		}
	}
	if assetURL == "" || checksumURL == "" {
		// Release exists but is missing an asset for this platform (e.g.
		// still uploading, or a malformed manual release) — nothing to
		// offer yet, but the check succeeded.
		return nil, saveUpdateCache(cache)
	}

	cache.LastNotifiedVersion = release.TagName
	if err := saveUpdateCache(cache); err != nil {
		return nil, err
	}

	return &updateState{Version: release.TagName, AssetURL: assetURL, ChecksumURL: checksumURL}, nil
}

// ---- download ----

const (
	downloadAssetTimeout     = 2 * time.Minute
	downloadChecksumsTimeout = 10 * time.Second
)

// downloadAsset streams the release asset at url to a new temp file and
// returns its path. On any failure — including a dropped connection
// mid-transfer — the partial temp file is removed and an empty path is
// returned, leaving the running executable untouched (spec: "Download
// fails mid-transfer").
func downloadAsset(ctx context.Context, url string) (path string, err error) {
	ctx, cancel := context.WithTimeout(ctx, downloadAssetTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: server returned %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "ngcode-update-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()

	if _, copyErr := io.Copy(tmp, resp.Body); copyErr != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("download interrupted: %w", copyErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		os.Remove(tmpPath)
		return "", closeErr
	}

	return tmpPath, nil
}

// downloadChecksums fetches and parses a checksums.txt in the standard
// sha256sum/"shasum -a 256" output format ("<hex>  <filename>" per line,
// whitespace-tolerant), exactly as produced by release.yml's "Generate
// checksums" step. Returns a map keyed by filename.
func downloadChecksums(ctx context.Context, url string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadChecksumsTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checksums download failed: server returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	sums := make(map[string]string)
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		// sha256sum's binary-mode output glues a "*" onto the filename
		// (confirmed on the Windows CI runner; macOS's shasum -a 256 never
		// does this) — strip it so the key matches assetNameForPlatform().
		name := strings.TrimPrefix(fields[1], "*")
		sums[name] = fields[0]
	}
	return sums, nil
}

// ---- write-permission probe ----

// checkWritePermission probes whether the process can write to dir by
// creating and removing a temp file inside it, WITHOUT touching the swap
// itself. This lets the accept flow fail fast, before any download, when
// an existing per-machine install (e.g. Program Files) lacks user write
// access — surfacing an explicit manual-reinstall message (app.go, PR3)
// instead of a late, confusing I/O error mid-swap.
func checkWritePermission(dir string) error {
	f, err := os.CreateTemp(dir, ".ngcode-update-probe-*")
	if err != nil {
		return fmt.Errorf("no write permission on %s: %w", dir, err)
	}
	path := f.Name()
	f.Close()
	return os.Remove(path)
}

// ---- old-binary cleanup ----

// executablePath is a seam over os.Executable so cleanupOldBinary and
// performSwap can be pointed at a fake path in tests instead of the real
// running test binary.
var executablePath = os.Executable

// cleanupOldBinary removes the "<executable>.old" file left behind by a
// previous Windows rename-dance swap, if present. Safe to call on every
// startup on every platform: a no-op when there is nothing to clean up.
// Deliberately NOT called from within performSwap itself — deleting
// "<exe>.old" immediately after a Windows rename-dance would race the
// still-exiting old process, which may still hold it open.
func cleanupOldBinary() error {
	exe, err := executablePath()
	if err != nil {
		return err
	}

	oldPath := exe + ".old"
	if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ---- platform-specific swap ----
//
// performSwap and its platform-specific helpers below are the ONE piece of
// this file NOT covered by go test. They coordinate with the real
// operating system to replace the currently-running executable — there is
// no safe way inside `go test` to exercise "replace my own running exe"
// without corrupting the test binary. This is documented, not silently
// skipped: verification is a MANUAL pre-release smoke test (tasks.md Phase
// 8.4) — build two versions, run the older one, trigger the update-accept
// flow, and confirm the app relaunches as the newer version on both a real
// Windows machine and a real macOS machine (including a Gatekeeper
// quarantine check on macOS, since the downloaded binary is unsigned:
// `xattr -d com.apple.quarantine` may be needed if relaunch is blocked).

// performSwap replaces the running executable with newBinaryPath. It is
// the literal last step of the accept flow — by the time it runs, checksum
// verification and the write-permission probe have both already passed.
// Dispatches by runtime.GOOS; any other platform is an explicit error.
func performSwap(newBinaryPath string) error {
	switch runtime.GOOS {
	case "windows":
		return performSwapWindows(newBinaryPath)
	case "darwin":
		return performSwapDarwin(newBinaryPath)
	default:
		return fmt.Errorf("auto-update swap is not supported on %s", runtime.GOOS)
	}
}

// performSwapWindows implements the rename-dance: Windows keeps an open
// handle to a running executable valid under rename (but not under
// overwrite/delete), so: current exe -> "<exe>.old", new binary -> the
// original path, then relaunch. Cleanup of "<exe>.old" happens on the NEXT
// startup via cleanupOldBinary(), not here — deleting it immediately would
// race the still-exiting old process.
func performSwapWindows(newBinaryPath string) error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	oldPath := exe + ".old"

	// Clear out any stale .old from a previous swap that was never
	// cleaned up, so the rename below doesn't fail with "already exists".
	_ = os.Remove(oldPath)

	if err := os.Rename(exe, oldPath); err != nil {
		return fmt.Errorf("rename current executable aside: %w", err)
	}
	if err := os.Rename(newBinaryPath, exe); err != nil {
		// Best-effort rollback so the app isn't left unable to start.
		_ = os.Rename(oldPath, exe)
		return fmt.Errorf("install new executable: %w", err)
	}

	return relaunch(exe)
}

// performSwapDarwin atomically replaces the .app bundle's inner Mach-O
// binary in place. macOS does not lock a running binary's inode against
// replacement the way Windows locks against overwrite, so a same-volume
// os.Rename is enough; if that fails (e.g. EXDEV because newBinaryPath is
// staged on a different volume than the install, such as /tmp), fall back
// to copy+rename. The fallback is attempted on ANY os.Rename failure
// rather than checking for a specific errno, to stay portable across
// platforms without importing platform-specific syscall constants.
func performSwapDarwin(newBinaryPath string) error {
	exe, err := executablePath()
	if err != nil {
		return err
	}

	if err := os.Rename(newBinaryPath, exe); err != nil {
		if fallbackErr := copyAndReplace(newBinaryPath, exe); fallbackErr != nil {
			return fmt.Errorf("replace executable: rename failed (%v), copy fallback failed (%w)", err, fallbackErr)
		}
	}

	return relaunch(exe)
}

// copyAndReplace copies src's contents into a temp file on dst's volume,
// makes it executable, then renames it over dst. Used only as the
// cross-device fallback for performSwapDarwin.
func copyAndReplace(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".ngcode-swap-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// relaunch spawns a new process of exe, preserving the original process's
// arguments, and returns — it does NOT call os.Exit or runtime.Quit
// itself. The caller (App.UpdateAccept, PR3) is responsible for quitting
// the current process via runtime.Quit(a.ctx) once relaunch succeeds,
// keeping this file free of any Wails runtime dependency.
func relaunch(exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}
