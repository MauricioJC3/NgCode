package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx           context.Context
	closeAnswerCh chan bool
	lsp           *lspClient
	version       string
	updater       *updateState
}

// FileData is the content of a file loaded into the editor
type FileData struct {
	Path    string
	Content string
}

// DirEntry is one immediate child of a directory listed in the file tree
type DirEntry struct {
	Name  string
	Path  string
	IsDir bool
}

// NewApp creates a new App application struct. version is the build's
// current version (injected via ldflags in CI, "dev" for local builds — see
// main.go) and is compared against GitHub's latest release by the
// background update check.
func NewApp(version string) *App {
	return &App{closeAnswerCh: make(chan bool), version: version}
}

// startup is called when the app starts. The context is saved so we can
// call the runtime methods. It also removes any leftover ".old" binary from
// a previous Windows rename-dance swap and kicks off a non-blocking
// background check for a newer release.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if err := cleanupOldBinary(); err != nil {
		runtime.LogError(ctx, "cleanupOldBinary: "+err.Error())
	}

	go a.checkForUpdates(ctx)
}

// checkForUpdates runs the background update check (updater.go) and, if a
// newer release is available, stores it on a.updater and notifies the
// frontend via the "update:available" event. A check error is logged only —
// per spec, a failed check aborts silently to the user and simply retries
// on the next launch (checkForUpdatesBackground already withholds the cache
// timestamp update in that case).
func (a *App) checkForUpdates(ctx context.Context) {
	state, err := checkForUpdatesBackground(ctx, a.version)
	if err != nil {
		runtime.LogError(ctx, "checkForUpdatesBackground: "+err.Error())
		return
	}
	if state == nil {
		return
	}

	a.updater = state
	runtime.EventsEmit(ctx, "update:available", UpdateInfo{
		Version:        state.Version,
		CurrentVersion: a.version,
	})
}

// beforeClose asks the frontend whether it's safe to close and blocks until it answers,
// since only the frontend knows which tabs have unsaved changes
func (a *App) beforeClose(ctx context.Context) bool {
	runtime.EventsEmit(ctx, "app:close-requested")
	shouldClose := <-a.closeAnswerCh
	return !shouldClose
}

// ConfirmClose answers a pending close request raised via the app:close-requested event
func (a *App) ConfirmClose(shouldClose bool) {
	a.closeAnswerCh <- shouldClose
}

// shutdown stops any running gopls process once the window is actually closing
func (a *App) shutdown(ctx context.Context) {
	if a.lsp != nil {
		_ = a.lsp.stop()
	}
}

// OpenFile prompts the user to pick a file and returns its content
func (a *App) OpenFile() (FileData, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Open File",
	})
	if err != nil {
		return FileData{}, err
	}
	if path == "" {
		return FileData{}, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return FileData{}, err
	}

	return FileData{Path: path, Content: string(content)}, nil
}

// SaveFile writes content to an existing path
func (a *App) SaveFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// SaveFileAs prompts the user for a destination and writes content to it
func (a *App) SaveFileAs(content string) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "Save File",
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}

	return path, nil
}

// OpenFolder prompts the user to pick a project root directory
func (a *App) OpenFolder() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Open Folder",
	})
}

// ReadDir lists the immediate children of path, directories first, each group alphabetical
func (a *App) ReadDir(path string) ([]DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var dirs, files []DirEntry
	for _, e := range entries {
		item := DirEntry{Name: e.Name(), Path: filepath.Join(path, e.Name()), IsDir: e.IsDir()}
		if e.IsDir() {
			dirs = append(dirs, item)
		} else {
			files = append(files, item)
		}
	}

	sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name) })
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })

	return append(dirs, files...), nil
}

// ReadFile reads a file's content by path, without a picker dialog
func (a *App) ReadFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// LspStart launches gopls rooted at rootPath, replacing any client already running
func (a *App) LspStart(rootPath string) error {
	if a.lsp != nil {
		_ = a.lsp.stop()
		a.lsp = nil
	}
	client, err := startLSPClient(a.ctx, rootPath)
	if err != nil {
		return err
	}
	a.lsp = client
	return nil
}

// LspStop shuts down the running gopls client, if any
func (a *App) LspStop() error {
	if a.lsp == nil {
		return nil
	}
	err := a.lsp.stop()
	a.lsp = nil
	return err
}

// LspDidOpen notifies gopls that path is now open with the given content
func (a *App) LspDidOpen(path string, content string) error {
	if a.lsp == nil {
		return nil
	}
	return a.lsp.didOpen(path, content)
}

// LspDidChange notifies gopls of the current full content of path
func (a *App) LspDidChange(path string, content string) error {
	if a.lsp == nil {
		return nil
	}
	return a.lsp.didChange(path, content)
}

// LspCompletion requests completions at a 0-indexed line/character position.
// Returns the raw LSP JSON result as a string (json.RawMessage confuses the
// Wails binding generator, which doesn't know that type and emits a broken import).
func (a *App) LspCompletion(path string, line int, character int) (string, error) {
	if a.lsp == nil {
		return "", nil
	}
	result, err := a.lsp.completion(path, line, character)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// LspDefinition requests the definition location(s) for the symbol at a
// 0-indexed line/character position. Returns the raw LSP JSON result as a
// string, same reasoning as LspCompletion.
func (a *App) LspDefinition(path string, line int, character int) (string, error) {
	if a.lsp == nil {
		return "", nil
	}
	result, err := a.lsp.definition(path, line, character)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// LspHover requests hover information for the symbol at a 0-indexed
// line/character position. Returns the raw LSP JSON result as a string,
// same reasoning as LspCompletion.
func (a *App) LspHover(path string, line int, character int) (string, error) {
	if a.lsp == nil {
		return "", nil
	}
	result, err := a.lsp.hover(path, line, character)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// UpdateAccept runs the full accept flow for the pending update stored in
// a.updater (populated by checkForUpdates): download the release asset,
// verify its SHA256 against checksums.txt, probe write permission on the
// install directory, then swap the running executable and relaunch.
// performSwap is the literal last step — every earlier gate must pass
// before anything destructive happens, so any error here leaves the
// running executable untouched (spec: "Download and Checksum
// Verification", "Permission-Aware Install"). The frontend surfaces a
// non-nil error the same way every other bound method's error is
// surfaced: console.error, no dedicated toast/error UI.
func (a *App) UpdateAccept() error {
	if a.updater == nil {
		return fmt.Errorf("no update available to accept")
	}
	state := a.updater

	assetPath, err := downloadAsset(a.ctx, state.AssetURL)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	// No-op once performSwap has consumed/renamed assetPath away on the
	// success path; cleans up the temp file on every earlier-return error.
	defer os.Remove(assetPath)

	checksums, err := downloadChecksums(a.ctx, state.ChecksumURL)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}

	assetName, err := assetNameForPlatform()
	if err != nil {
		return err
	}
	wantHex, ok := checksums[assetName]
	if !ok {
		return fmt.Errorf("no checksum published for %s", assetName)
	}
	if err := verifyChecksum(assetPath, wantHex); err != nil {
		return err
	}

	exe, err := executablePath()
	if err != nil {
		return err
	}
	if err := checkWritePermission(filepath.Dir(exe)); err != nil {
		return fmt.Errorf("insufficient permission to install update, please reinstall manually: %w", err)
	}

	if err := performSwap(assetPath); err != nil {
		return err
	}

	a.updater = nil
	runtime.Quit(a.ctx)
	return nil
}

// UpdateDismiss discards the pending update without persisting anything.
// The update-check cadence cache (updater.go) is untouched, so per spec
// there is no "remember dismiss" — the user is asked again on the next
// successful background check.
func (a *App) UpdateDismiss() {
	a.updater = nil
}
