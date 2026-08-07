package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- parseSemver / compareSemver (tasks.md 2.1) ----

func TestParseSemver(t *testing.T) {
	cases := []struct {
		name                            string
		in                              string
		wantMajor, wantMinor, wantPatch int
		wantErr                         bool
	}{
		{"with v prefix", "v1.2.3", 1, 2, 3, false},
		{"without v prefix", "1.2.3", 1, 2, 3, false},
		{"zeroes", "v0.0.0", 0, 0, 0, false},
		{"missing component", "v1.2", 0, 0, 0, true},
		{"non-numeric component", "v1.x.3", 0, 0, 0, true},
		{"empty string", "", 0, 0, 0, true},
		{"extra component", "v1.2.3.4", 0, 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			major, minor, patch, err := parseSemver(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseSemver(%q): want error, got nil", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSemver(%q): unexpected error: %v", tc.in, err)
			}
			if major != tc.wantMajor || minor != tc.wantMinor || patch != tc.wantPatch {
				t.Fatalf("parseSemver(%q) = %d.%d.%d, want %d.%d.%d", tc.in, major, minor, patch, tc.wantMajor, tc.wantMinor, tc.wantPatch)
			}
		})
	}
}

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "v1.2.3", "v1.2.3", 0},
		{"a older major", "v1.2.3", "v2.0.0", -1},
		{"a newer major", "v2.0.0", "v1.2.3", 1},
		{"a older minor", "v1.1.9", "v1.2.0", -1},
		{"a newer patch", "v1.2.3", "v1.2.2", 1},
		{"a malformed, b valid", "not-a-version", "v1.0.0", -1},
		{"a valid, b malformed", "v1.0.0", "not-a-version", 1},
		{"both malformed", "nope", "also-nope", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compareSemver(tc.a, tc.b)
			if got != tc.want {
				t.Fatalf("compareSemver(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// ---- assetNameForPlatform (tasks.md 2.2) ----
// Naming MUST match .github/workflows/release.yml's matrix.asset_name
// values byte-for-byte (verified against the file on disk, PR1 branch
// feat/auto-update-ci-config, commit 0ea40d8).

func TestAssetNameForPlatform(t *testing.T) {
	origGOOS, origGOARCH := goos, goarch
	t.Cleanup(func() { goos, goarch = origGOOS, origGOARCH })

	cases := []struct {
		name         string
		goos, goarch string
		want         string
		wantErr      bool
	}{
		{"windows amd64", "windows", "amd64", "ngcode-windows-amd64.exe", false},
		{"darwin amd64", "darwin", "amd64", "ngcode-darwin-amd64", false},
		{"darwin arm64", "darwin", "arm64", "ngcode-darwin-arm64", false},
		{"windows arm64 unsupported (release.yml does not build this)", "windows", "arm64", "", true},
		{"linux amd64 unsupported (release.yml does not build this)", "linux", "amd64", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goos, goarch = tc.goos, tc.goarch
			got, err := assetNameForPlatform()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("assetNameForPlatform(): want error for %s/%s, got nil", tc.goos, tc.goarch)
				}
				return
			}
			if err != nil {
				t.Fatalf("assetNameForPlatform(): unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("assetNameForPlatform() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---- update-check.json cache (tasks.md 2.3) ----
// os.UserConfigDir() on Linux honors $XDG_CONFIG_HOME, which is what this
// project's test runner (go test ./..., Linux/WSL) can reliably isolate
// per-test via t.Setenv.

func TestUpdateCache(t *testing.T) {
	t.Run("missing file returns zero-value never-checked cache", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		cache, err := loadUpdateCache()
		if err != nil {
			t.Fatalf("loadUpdateCache(): unexpected error: %v", err)
		}
		if !cache.LastCheck.IsZero() {
			t.Fatalf("loadUpdateCache(): want zero-value LastCheck, got %v", cache.LastCheck)
		}
		if !shouldCheckForUpdates(cache, time.Now()) {
			t.Fatalf("shouldCheckForUpdates(): want true for a never-checked cache")
		}
	})

	t.Run("corrupt JSON treated as never-checked, not an error", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)

		path, err := updateCacheFilePath()
		if err != nil {
			t.Fatalf("updateCacheFilePath(): unexpected error: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		cache, err := loadUpdateCache()
		if err != nil {
			t.Fatalf("loadUpdateCache(): unexpected error on corrupt cache: %v", err)
		}
		if !cache.LastCheck.IsZero() {
			t.Fatalf("loadUpdateCache(): want zero-value on corrupt cache, got %v", cache.LastCheck)
		}
	})

	t.Run("check skipped inside the 24h window", func(t *testing.T) {
		cache := updateCache{LastCheck: time.Now().Add(-1 * time.Hour)}
		if shouldCheckForUpdates(cache, time.Now()) {
			t.Fatalf("shouldCheckForUpdates(): want false inside the 24h window")
		}
	})

	t.Run("check runs after the 24h window elapses", func(t *testing.T) {
		cache := updateCache{LastCheck: time.Now().Add(-25 * time.Hour)}
		if !shouldCheckForUpdates(cache, time.Now()) {
			t.Fatalf("shouldCheckForUpdates(): want true after the 24h window")
		}
	})

	t.Run("round trip save then load", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		want := updateCache{LastCheck: time.Now().Truncate(time.Second).UTC(), LastNotifiedVersion: "v1.2.3"}
		if err := saveUpdateCache(want); err != nil {
			t.Fatalf("saveUpdateCache(): unexpected error: %v", err)
		}
		got, err := loadUpdateCache()
		if err != nil {
			t.Fatalf("loadUpdateCache(): unexpected error: %v", err)
		}
		if !got.LastCheck.Equal(want.LastCheck) || got.LastNotifiedVersion != want.LastNotifiedVersion {
			t.Fatalf("round trip mismatch: got %+v, want %+v", got, want)
		}
	})
}

// ---- verifyChecksum (tasks.md 2.4) ----

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asset.bin")
	content := []byte("ngcode auto-update fixture content")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	sum := sha256.Sum256(content)
	wantHex := hex.EncodeToString(sum[:])

	t.Run("matching checksum", func(t *testing.T) {
		if err := verifyChecksum(path, wantHex); err != nil {
			t.Fatalf("verifyChecksum(): unexpected error: %v", err)
		}
	})

	t.Run("matching checksum is case-insensitive", func(t *testing.T) {
		if err := verifyChecksum(path, strings.ToUpper(wantHex)); err != nil {
			t.Fatalf("verifyChecksum(): unexpected error with uppercase hex: %v", err)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		bogus := strings.Repeat("0", len(wantHex))
		if err := verifyChecksum(path, bogus); err == nil {
			t.Fatalf("verifyChecksum(): want error on mismatch, got nil")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if err := verifyChecksum(filepath.Join(dir, "does-not-exist.bin"), wantHex); err == nil {
			t.Fatalf("verifyChecksum(): want error for a missing file, got nil")
		}
	})
}

// ---- checkForUpdatesBackground (tasks.md 3.1) ----

func swapGithubReleasesURL(u string) (restore func()) {
	orig := githubReleasesURL
	githubReleasesURL = u
	return func() { githubReleasesURL = orig }
}

func swapPlatform(goosVal, goarchVal string) (restore func()) {
	origGOOS, origGOARCH := goos, goarch
	goos, goarch = goosVal, goarchVal
	return func() { goos, goarch = origGOOS, origGOARCH }
}

func TestCheckForUpdatesBackground(t *testing.T) {
	t.Run("newer version available returns update state and persists cache", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"tag_name": "v9.9.9",
				"assets": [
					{"name": "ngcode-windows-amd64.exe", "browser_download_url": "https://example.invalid/ngcode-windows-amd64.exe"},
					{"name": "checksums.txt", "browser_download_url": "https://example.invalid/checksums.txt"}
				]
			}`))
		}))
		defer srv.Close()
		defer swapGithubReleasesURL(srv.URL)()
		defer swapPlatform("windows", "amd64")()
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		state, err := checkForUpdatesBackground(context.Background(), "v1.0.0")
		if err != nil {
			t.Fatalf("checkForUpdatesBackground(): unexpected error: %v", err)
		}
		if state == nil {
			t.Fatalf("checkForUpdatesBackground(): want non-nil state for a newer version")
		}
		if state.Version != "v9.9.9" {
			t.Fatalf("state.Version = %q, want v9.9.9", state.Version)
		}
		if state.AssetURL != "https://example.invalid/ngcode-windows-amd64.exe" {
			t.Fatalf("state.AssetURL = %q", state.AssetURL)
		}
		if state.ChecksumURL != "https://example.invalid/checksums.txt" {
			t.Fatalf("state.ChecksumURL = %q", state.ChecksumURL)
		}

		cache, err := loadUpdateCache()
		if err != nil {
			t.Fatalf("loadUpdateCache(): unexpected error: %v", err)
		}
		if cache.LastCheck.IsZero() {
			t.Fatalf("want cache.LastCheck persisted after a successful check")
		}
	})

	t.Run("already on latest version returns nil state, no error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name": "v1.0.0", "assets": []}`))
		}))
		defer srv.Close()
		defer swapGithubReleasesURL(srv.URL)()
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		state, err := checkForUpdatesBackground(context.Background(), "v1.0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state != nil {
			t.Fatalf("want nil state when already on latest, got %+v", state)
		}
	})

	t.Run("check skipped inside the 24h window makes no network call", func(t *testing.T) {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			_, _ = w.Write([]byte(`{"tag_name": "v9.9.9", "assets": []}`))
		}))
		defer srv.Close()
		defer swapGithubReleasesURL(srv.URL)()

		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		if err := saveUpdateCache(updateCache{LastCheck: time.Now()}); err != nil {
			t.Fatalf("saveUpdateCache: %v", err)
		}

		state, err := checkForUpdatesBackground(context.Background(), "v1.0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if state != nil {
			t.Fatalf("want nil state, got %+v", state)
		}
		if called {
			t.Fatalf("want no network call inside the 24h window")
		}
	})

	t.Run("rate-limited response returns error and does not update the cache", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer srv.Close()
		defer swapGithubReleasesURL(srv.URL)()
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		if _, err := checkForUpdatesBackground(context.Background(), "v1.0.0"); err == nil {
			t.Fatalf("want error on a 403 response")
		}

		cache, err := loadUpdateCache()
		if err != nil {
			t.Fatalf("loadUpdateCache: %v", err)
		}
		if !cache.LastCheck.IsZero() {
			t.Fatalf("want cache.LastCheck untouched on a failed check so the next launch retries, got %v", cache.LastCheck)
		}
	})

	t.Run("offline / connection refused returns error", func(t *testing.T) {
		defer swapGithubReleasesURL("http://127.0.0.1:1")() // nothing listens on this port
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())

		if _, err := checkForUpdatesBackground(context.Background(), "v1.0.0"); err == nil {
			t.Fatalf("want error when the API is unreachable")
		}
	})
}

// ---- downloadAsset / downloadChecksums (tasks.md 3.2) ----

func TestDownloadAsset(t *testing.T) {
	t.Run("successful download writes a temp file with matching content", func(t *testing.T) {
		want := []byte("fake binary payload")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(want)
		}))
		defer srv.Close()

		path, err := downloadAsset(context.Background(), srv.URL)
		if err != nil {
			t.Fatalf("downloadAsset(): unexpected error: %v", err)
		}
		defer os.Remove(path)

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("downloaded content = %q, want %q", got, want)
		}
	})

	t.Run("server error aborts and returns no path", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		path, err := downloadAsset(context.Background(), srv.URL)
		if err == nil {
			t.Fatalf("want error on a server 500, got nil (path=%q)", path)
		}
		if path != "" {
			t.Fatalf("want an empty path on error, got %q", path)
		}
	})

	t.Run("dropped connection discards partial data", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "1000000")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("only a few bytes"))
			hj, ok := w.(http.Hijacker)
			if !ok {
				return
			}
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()
			}
		}))
		defer srv.Close()

		path, err := downloadAsset(context.Background(), srv.URL)
		if err == nil {
			t.Fatalf("want error on a dropped connection, got nil (path=%q)", path)
		}
		if path != "" {
			t.Fatalf("want an empty path on an aborted download, got %q", path)
		}
	})
}

func TestDownloadChecksums(t *testing.T) {
	t.Run("parses sha256sum-format output", func(t *testing.T) {
		body := "aaaa000000000000000000000000000000000000000000000000000000000000  ngcode-windows-amd64.exe\n" +
			"bbbb111111111111111111111111111111111111111111111111111111111111  ngcode-darwin-amd64\n"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		defer srv.Close()

		sums, err := downloadChecksums(context.Background(), srv.URL)
		if err != nil {
			t.Fatalf("downloadChecksums(): unexpected error: %v", err)
		}
		if sums["ngcode-windows-amd64.exe"] != "aaaa000000000000000000000000000000000000000000000000000000000000" {
			t.Fatalf("unexpected checksum for the windows asset: %q", sums["ngcode-windows-amd64.exe"])
		}
		if sums["ngcode-darwin-amd64"] != "bbbb111111111111111111111111111111111111111111111111111111111111" {
			t.Fatalf("unexpected checksum for the darwin asset: %q", sums["ngcode-darwin-amd64"])
		}
	})

	t.Run("server error returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		if _, err := downloadChecksums(context.Background(), srv.URL); err == nil {
			t.Fatalf("want error on a 404 response")
		}
	})
}

// ---- checkWritePermission (tasks.md 3.3) ----

func TestCheckWritePermission(t *testing.T) {
	t.Run("writable directory succeeds", func(t *testing.T) {
		if err := checkWritePermission(t.TempDir()); err != nil {
			t.Fatalf("checkWritePermission(): unexpected error on a writable dir: %v", err)
		}
	})

	t.Run("read-only directory fails", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("running as root: permission checks are not enforced (no permission control on this runner)")
		}
		dir := t.TempDir()
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		defer os.Chmod(dir, 0o755) // restore write access so t.TempDir() cleanup succeeds

		if err := checkWritePermission(dir); err == nil {
			t.Fatalf("checkWritePermission(): want error on a read-only dir, got nil")
		}
	})
}

// ---- cleanupOldBinary (tasks.md 3.4) ----

func TestCleanupOldBinary(t *testing.T) {
	origExecutablePath := executablePath
	t.Cleanup(func() { executablePath = origExecutablePath })

	t.Run("old binary present is removed", func(t *testing.T) {
		dir := t.TempDir()
		exe := filepath.Join(dir, "ngcode")
		executablePath = func() (string, error) { return exe, nil }

		oldPath := exe + ".old"
		if err := os.WriteFile(oldPath, []byte("stale"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		if err := cleanupOldBinary(); err != nil {
			t.Fatalf("cleanupOldBinary(): unexpected error: %v", err)
		}
		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Fatalf("want %s removed, stat err = %v", oldPath, err)
		}
	})

	t.Run("no old binary present is a no-op", func(t *testing.T) {
		dir := t.TempDir()
		exe := filepath.Join(dir, "ngcode")
		executablePath = func() (string, error) { return exe, nil }

		if err := cleanupOldBinary(); err != nil {
			t.Fatalf("cleanupOldBinary(): unexpected error on no-op: %v", err)
		}
	})
}

// ---- performSwap (tasks.md 4.1) ----
//
// NOT COVERED HERE — intentionally. performSwap coordinates with the real
// operating system to replace the currently-running executable (Windows
// rename-dance / macOS atomic binary replace inside the .app bundle). There
// is no safe way to exercise "replace my own running exe" inside `go test`
// without either corrupting the test binary or requiring root/signing
// setup that doesn't exist in this environment. This is called out
// explicitly in the design and tasks artifacts as the one piece of Phase 4
// requiring a MANUAL smoke test (tasks.md Phase 8.4) before each release:
// build two versions, run the older one, trigger the update-accept flow,
// and confirm the app relaunches as the newer version on both a real
// Windows machine and a real macOS machine (including a Gatekeeper
// quarantine check on macOS for the unsigned downloaded binary).
