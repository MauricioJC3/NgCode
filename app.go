package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx context.Context

	// lspClients holds at most one running client per language (keyed by
	// the lspServers registry key in lsp.go — "go", "typescript", "json",
	// "css"), started lazily the first time a file of that language is
	// opened (see LspEnsureStarted).
	lspMu      sync.RWMutex
	lspClients map[string]*lspClient

	updaterMu sync.Mutex
	version   string
	updater   *updateState
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
	return &App{lspClients: make(map[string]*lspClient), version: version}
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

	a.updaterMu.Lock()
	a.updater = state
	a.updaterMu.Unlock()
	runtime.EventsEmit(ctx, "update:available", UpdateInfo{
		Version:        state.Version,
		CurrentVersion: a.version,
	})
}

// beforeClose always vetoes the native window close (returns true) and asks
// the frontend, via an event, whether it's actually safe to close — only the
// frontend knows which tabs have unsaved changes.
//
// It must NEVER block waiting for that answer. Per Wails v2's Windows
// frontend (mainWindow.OnClose().Bind(...) -> f.Quit() -> OnBeforeClose),
// this runs SYNCHRONOUSLY on the same OS thread that pumps the window's
// message loop — the thread WebView2 itself needs to deliver the frontend's
// JS->Go round trip (the ForceQuit call below) back into this process. A
// version of this that blocked on a channel waiting for that round trip
// (the previous implementation) therefore self-deadlocked on every close:
// the UI thread can't process the incoming call while it's stuck waiting
// for it. That surfaced as Windows reporting the app "not responding" on
// every single close, not just the update-related case that first exposed
// it. ForceQuit is what actually terminates the process, once the frontend
// has decided it's safe to.
func (a *App) beforeClose(ctx context.Context) bool {
	runtime.EventsEmit(ctx, "app:close-requested")
	return true
}

// ForceQuit is called by the frontend once it has confirmed the app is safe
// to close (no unsaved changes, or the user accepted losing them). It must
// NOT go through runtime.Quit()/beforeClose again — that would just veto
// itself the same way. Stops every running language server first: Windows
// does not kill child processes when their parent exits, so skipping this
// would leak an orphaned gopls.exe/typescript-language-server.exe/etc. on
// every close.
func (a *App) ForceQuit() {
	for _, client := range a.stopAllLSP() {
		_ = client.stop()
	}
	os.Exit(0)
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

// ReadDir lists the immediate children of path, directories first, each group alphabetical.
// dirs/files start as non-nil empty slices (not `var dirs, files []DirEntry`) so an empty
// directory serializes to JSON `[]` instead of `null` — the frontend calls .forEach on the
// result, which throws on null and silently aborts opening the folder.
func (a *App) ReadDir(path string) ([]DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	dirs := []DirEntry{}
	files := []DirEntry{}
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

// validEntryName rejects empty names, path separators, and "." / ".." — a
// name that could otherwise make CreateFile/CreateFolder escape dirPath or
// silently no-op.
func validEntryName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid name")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("name cannot contain path separators")
	}
	return nil
}

// CreateFile creates a new empty file at dirPath/name and returns its full
// path. Fails (rather than truncating) if something already exists there,
// since this backs an explicit "New File" action.
func (a *App) CreateFile(dirPath string, name string) (string, error) {
	if err := validEntryName(name); err != nil {
		return "", err
	}
	path := filepath.Join(dirPath, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	f.Close()
	return path, nil
}

// CreateFolder creates a new directory at dirPath/name and returns its full path.
func (a *App) CreateFolder(dirPath string, name string) (string, error) {
	if err := validEntryName(name); err != nil {
		return "", err
	}
	path := filepath.Join(dirPath, name)
	if err := os.Mkdir(path, 0755); err != nil {
		return "", err
	}
	return path, nil
}

// moveOrRename performs the collision check + os.Rename tail shared by
// MoveEntry (dropping an entry into a different folder, same base name) and
// RenameEntry (giving an entry a new base name, same parent folder). Rejects
// a name collision at destPath rather than silently overwriting. Callers own
// any same-path/same-name guard themselves — MoveEntry's "already in that
// folder" and RenameEntry's "already named that" need different wording for
// what is the same underlying destPath == srcPath condition, so a shared
// helper can't own that copy (design: "moveOrRename scope").
func moveOrRename(srcPath string, destPath string) (string, error) {
	if _, err := os.Stat(destPath); err == nil {
		return "", fmt.Errorf("%s already exists", filepath.Base(destPath))
	}

	if err := os.Rename(srcPath, destPath); err != nil {
		return "", err
	}
	return destPath, nil
}

// isWorkspaceRoot reports whether path refers to the same location as
// workspaceRoot, after cleaning both (trailing slashes, "..", relative
// segments). Used as a per-call guard on DeleteEntry/RenameEntry — matches
// the existing LspEnsureStarted/SearchInWorkspace convention of passing
// workspaceRoot per call instead of storing it on App (design: "Workspace-
// root guard state").
func isWorkspaceRoot(path string, workspaceRoot string) bool {
	return filepath.Clean(path) == filepath.Clean(workspaceRoot)
}

// MoveEntry moves the file or folder at srcPath into destDir (keeping its
// base name) and returns the new path — backs drag-and-drop in the file
// tree. Rejects moving a folder into itself or one of its own descendants
// (would otherwise let os.Rename corrupt the tree), and rejects a name
// collision at the destination rather than silently overwriting.
func (a *App) MoveEntry(srcPath string, destDir string) (string, error) {
	srcPath = filepath.Clean(srcPath)
	destDir = filepath.Clean(destDir)

	info, err := os.Stat(srcPath)
	if err != nil {
		return "", err
	}
	destInfo, err := os.Stat(destDir)
	if err != nil {
		return "", err
	}
	if !destInfo.IsDir() {
		return "", fmt.Errorf("destination is not a folder")
	}

	if info.IsDir() {
		if rel, relErr := filepath.Rel(srcPath, destDir); relErr == nil &&
			rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("cannot move a folder into itself or one of its own subfolders")
		}
	}

	destPath := filepath.Join(destDir, filepath.Base(srcPath))
	if destPath == srcPath {
		return "", fmt.Errorf("already in that folder")
	}

	return moveOrRename(srcPath, destPath)
}

// DeleteEntry permanently removes the file or folder at path — backs the
// explorer's context-menu Delete item and Delete key. Uses os.RemoveAll for
// both files and folders (RemoveAll on a file behaves identically to
// os.Remove, so this is one code path rather than branching on IsDir —
// design: "Delete semantics"). Rejects deleting the workspace root itself.
func (a *App) DeleteEntry(path string, workspaceRoot string) error {
	if isWorkspaceRoot(path, workspaceRoot) {
		return fmt.Errorf("cannot delete the workspace root")
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

// RenameEntry renames the file or folder at oldPath to newName, keeping it
// in the same parent folder, and returns the new full path — backs the
// explorer's context-menu Rename item and F2 key. Reuses moveOrRename for
// the collision-check + os.Rename tail (shared with MoveEntry). Note:
// remapping open tab paths for a renamed folder is a frontend concern, not
// this method's — RenameEntry only returns the new path.
func (a *App) RenameEntry(oldPath string, newName string, workspaceRoot string) (string, error) {
	if err := validEntryName(newName); err != nil {
		return "", err
	}
	if isWorkspaceRoot(oldPath, workspaceRoot) {
		return "", fmt.Errorf("cannot rename the workspace root")
	}
	if newName == filepath.Base(oldPath) {
		return "", fmt.Errorf("already named that")
	}

	destPath := filepath.Join(filepath.Dir(oldPath), newName)
	return moveOrRename(oldPath, destPath)
}

// currentLSP returns the running client for lang, if any, under lspMu — the
// single point where lspClients is read so every caller sees a consistent
// snapshot instead of racing LspEnsureStarted/LspStopAll on the raw map.
func (a *App) currentLSP(lang string) *lspClient {
	a.lspMu.RLock()
	defer a.lspMu.RUnlock()
	return a.lspClients[lang]
}

// stopAllLSP atomically clears lspClients and returns the clients that were
// running, leaving the actual (slow, per-process) shutdown to the caller.
// Shared by LspStopAll and ForceQuit so both go through the same swap.
func (a *App) stopAllLSP() []*lspClient {
	a.lspMu.Lock()
	clients := a.lspClients
	a.lspClients = make(map[string]*lspClient)
	a.lspMu.Unlock()

	out := make([]*lspClient, 0, len(clients))
	for _, c := range clients {
		out = append(out, c)
	}
	return out
}

// LspEnsureStarted makes sure a language server for path's language is
// running, rooted at rootPath, starting one via the lspServers registry
// (lsp.go) if none is running yet. No-op for a path whose extension isn't
// backed by any configured server (languageForPath returns ""). Servers are
// started lazily, on first file-open of their language, rather than eagerly
// per workspace — avoids spawning e.g. typescript-language-server for a
// workspace nobody is editing TypeScript in.
func (a *App) LspEnsureStarted(rootPath string, path string) error {
	lang := languageForPath(path)
	if lang == "" {
		return nil
	}

	a.lspMu.RLock()
	_, running := a.lspClients[lang]
	a.lspMu.RUnlock()
	if running {
		return nil
	}

	spec, ok := lspServers[lang]
	if !ok {
		return nil
	}
	client, err := startLSPClient(a.ctx, rootPath, spec)
	if err != nil {
		return err
	}

	a.lspMu.Lock()
	a.lspClients[lang] = client
	a.lspMu.Unlock()
	return nil
}

// LspStopAll shuts down every running language server — called when
// switching workspace root, since every client is rooted at the folder that
// was open when it started.
func (a *App) LspStopAll() error {
	for _, client := range a.stopAllLSP() {
		_ = client.stop()
	}
	return nil
}

// LspDidOpen notifies path's language server that it's now open with the given content
func (a *App) LspDidOpen(path string, content string) error {
	client := a.currentLSP(languageForPath(path))
	if client == nil {
		return nil
	}
	return client.didOpen(path, content)
}

// LspDidChange notifies path's language server of its current full content
func (a *App) LspDidChange(path string, content string) error {
	client := a.currentLSP(languageForPath(path))
	if client == nil {
		return nil
	}
	return client.didChange(path, content)
}

// LspCompletion requests completions at a 0-indexed line/character position.
// Returns the raw LSP JSON result as a string (json.RawMessage confuses the
// Wails binding generator, which doesn't know that type and emits a broken import).
func (a *App) LspCompletion(path string, line int, character int) (string, error) {
	client := a.currentLSP(languageForPath(path))
	if client == nil {
		return "", nil
	}
	result, err := client.completion(path, line, character)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// LspDefinition requests the definition location(s) for the symbol at a
// 0-indexed line/character position. Returns the raw LSP JSON result as a
// string, same reasoning as LspCompletion.
func (a *App) LspDefinition(path string, line int, character int) (string, error) {
	client := a.currentLSP(languageForPath(path))
	if client == nil {
		return "", nil
	}
	result, err := client.definition(path, line, character)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// LspHover requests hover information for the symbol at a 0-indexed
// line/character position. Returns the raw LSP JSON result as a string,
// same reasoning as LspCompletion.
func (a *App) LspHover(path string, line int, character int) (string, error) {
	client := a.currentLSP(languageForPath(path))
	if client == nil {
		return "", nil
	}
	result, err := client.hover(path, line, character)
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
	a.updaterMu.Lock()
	state := a.updater
	a.updaterMu.Unlock()
	if state == nil {
		return fmt.Errorf("no update available to accept")
	}

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

	a.updaterMu.Lock()
	a.updater = nil
	a.updaterMu.Unlock()

	// performSwap already renamed the new binary into place and launched it
	// as a separate process (relaunch, updater.go) — the replacement is
	// already running by this point, so there's nothing left to negotiate.
	// os.Exit (not runtime.Quit) matches ForceQuit's reasoning below: get
	// out of the way immediately instead of going through Wails' close
	// flow, which would otherwise leave this still-shutting-down process
	// and the freshly-launched one contending for the same default
	// WebView2 profile directory.
	os.Exit(0)
	return nil
}

// UpdateDismiss discards the pending update without persisting anything.
// The update-check cadence cache (updater.go) is untouched, so per spec
// there is no "remember dismiss" — the user is asked again on the next
// successful background check.
func (a *App) UpdateDismiss() {
	a.updaterMu.Lock()
	a.updater = nil
	a.updaterMu.Unlock()
}
