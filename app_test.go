package main

import (
	"os"
	"path/filepath"
	"testing"
)

// --- MoveEntry ------------------------------------------------------------
//
// Regression suite capturing MoveEntry's behavior BEFORE the moveOrRename
// extraction (see design's "moveOrRename extraction" — MoveEntry's
// collision-check + os.Rename tail is being pulled into a shared helper
// reused by RenameEntry). This suite must stay green across that refactor;
// it is the safety net, not new behavior.

func TestMoveEntry(t *testing.T) {
	t.Run("moves a file into a destination folder", func(t *testing.T) {
		dir := t.TempDir()
		srcDir := filepath.Join(dir, "src")
		destDir := filepath.Join(dir, "dest")
		if err := os.Mkdir(srcDir, 0755); err != nil {
			t.Fatalf("mkdir src: %v", err)
		}
		if err := os.Mkdir(destDir, 0755); err != nil {
			t.Fatalf("mkdir dest: %v", err)
		}
		srcPath := filepath.Join(srcDir, "file.txt")
		if err := os.WriteFile(srcPath, []byte("hello"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		app := &App{}
		newPath, err := app.MoveEntry(srcPath, destDir)
		if err != nil {
			t.Fatalf("MoveEntry: unexpected error: %v", err)
		}
		wantPath := filepath.Join(destDir, "file.txt")
		if newPath != wantPath {
			t.Errorf("MoveEntry returned %q, want %q", newPath, wantPath)
		}
		if _, err := os.Stat(wantPath); err != nil {
			t.Errorf("expected moved file at %q: %v", wantPath, err)
		}
		if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
			t.Errorf("expected src path %q to no longer exist, got err=%v", srcPath, err)
		}
	})

	t.Run("moves a folder into a destination folder with nested contents intact", func(t *testing.T) {
		dir := t.TempDir()
		srcFolder := filepath.Join(dir, "srcfolder")
		destDir := filepath.Join(dir, "dest")
		if err := os.MkdirAll(filepath.Join(srcFolder, "nested"), 0755); err != nil {
			t.Fatalf("mkdir nested: %v", err)
		}
		if err := os.Mkdir(destDir, 0755); err != nil {
			t.Fatalf("mkdir dest: %v", err)
		}
		nestedFile := filepath.Join(srcFolder, "nested", "inner.txt")
		if err := os.WriteFile(nestedFile, []byte("data"), 0644); err != nil {
			t.Fatalf("write nested file: %v", err)
		}

		app := &App{}
		newPath, err := app.MoveEntry(srcFolder, destDir)
		if err != nil {
			t.Fatalf("MoveEntry: unexpected error: %v", err)
		}
		wantPath := filepath.Join(destDir, "srcfolder")
		if newPath != wantPath {
			t.Errorf("MoveEntry returned %q, want %q", newPath, wantPath)
		}
		wantNested := filepath.Join(wantPath, "nested", "inner.txt")
		content, err := os.ReadFile(wantNested)
		if err != nil {
			t.Fatalf("expected nested file at %q: %v", wantNested, err)
		}
		if string(content) != "data" {
			t.Errorf("nested file content = %q, want %q", string(content), "data")
		}
	})

	t.Run("rejects moving a folder into itself", func(t *testing.T) {
		dir := t.TempDir()
		srcFolder := filepath.Join(dir, "srcfolder")
		if err := os.Mkdir(srcFolder, 0755); err != nil {
			t.Fatalf("mkdir src: %v", err)
		}

		app := &App{}
		_, err := app.MoveEntry(srcFolder, srcFolder)
		if err == nil {
			t.Fatal("expected error moving a folder into itself, got nil")
		}
	})

	t.Run("rejects moving a folder into one of its own descendants", func(t *testing.T) {
		dir := t.TempDir()
		srcFolder := filepath.Join(dir, "srcfolder")
		descendant := filepath.Join(srcFolder, "child")
		if err := os.MkdirAll(descendant, 0755); err != nil {
			t.Fatalf("mkdir descendant: %v", err)
		}

		app := &App{}
		_, err := app.MoveEntry(srcFolder, descendant)
		if err == nil {
			t.Fatal("expected error moving a folder into its own descendant, got nil")
		}
	})

	t.Run("rejects destPath == srcPath (already in that folder)", func(t *testing.T) {
		dir := t.TempDir()
		parent := filepath.Join(dir, "parent")
		if err := os.Mkdir(parent, 0755); err != nil {
			t.Fatalf("mkdir parent: %v", err)
		}
		srcPath := filepath.Join(parent, "file.txt")
		if err := os.WriteFile(srcPath, []byte("hi"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		app := &App{}
		_, err := app.MoveEntry(srcPath, parent)
		if err == nil {
			t.Fatal("expected error moving into the same parent folder, got nil")
		}
		wantMsg := "already in that folder"
		if err.Error() != wantMsg {
			t.Errorf("error = %q, want %q", err.Error(), wantMsg)
		}
	})

	t.Run("rejects name collision at destination", func(t *testing.T) {
		dir := t.TempDir()
		srcDir := filepath.Join(dir, "src")
		destDir := filepath.Join(dir, "dest")
		if err := os.Mkdir(srcDir, 0755); err != nil {
			t.Fatalf("mkdir src: %v", err)
		}
		if err := os.Mkdir(destDir, 0755); err != nil {
			t.Fatalf("mkdir dest: %v", err)
		}
		srcPath := filepath.Join(srcDir, "file.txt")
		if err := os.WriteFile(srcPath, []byte("hi"), 0644); err != nil {
			t.Fatalf("write src file: %v", err)
		}
		collidingPath := filepath.Join(destDir, "file.txt")
		if err := os.WriteFile(collidingPath, []byte("existing"), 0644); err != nil {
			t.Fatalf("write colliding file: %v", err)
		}

		app := &App{}
		_, err := app.MoveEntry(srcPath, destDir)
		if err == nil {
			t.Fatal("expected error on name collision, got nil")
		}
		wantMsg := "file.txt already exists"
		if err.Error() != wantMsg {
			t.Errorf("error = %q, want %q", err.Error(), wantMsg)
		}
	})

	t.Run("errors when srcPath does not exist", func(t *testing.T) {
		dir := t.TempDir()
		destDir := filepath.Join(dir, "dest")
		if err := os.Mkdir(destDir, 0755); err != nil {
			t.Fatalf("mkdir dest: %v", err)
		}
		srcPath := filepath.Join(dir, "does-not-exist.txt")

		app := &App{}
		_, err := app.MoveEntry(srcPath, destDir)
		if err == nil {
			t.Fatal("expected error for nonexistent srcPath, got nil")
		}
	})

	t.Run("errors when destDir does not exist", func(t *testing.T) {
		dir := t.TempDir()
		srcPath := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(srcPath, []byte("hi"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		destDir := filepath.Join(dir, "does-not-exist")

		app := &App{}
		_, err := app.MoveEntry(srcPath, destDir)
		if err == nil {
			t.Fatal("expected error for nonexistent destDir, got nil")
		}
	})

	t.Run("errors when destDir is not a folder", func(t *testing.T) {
		dir := t.TempDir()
		srcPath := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(srcPath, []byte("hi"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
		notAFolder := filepath.Join(dir, "notafolder.txt")
		if err := os.WriteFile(notAFolder, []byte("nope"), 0644); err != nil {
			t.Fatalf("write notAFolder: %v", err)
		}

		app := &App{}
		_, err := app.MoveEntry(srcPath, notAFolder)
		if err == nil {
			t.Fatal("expected error for destDir that is not a folder, got nil")
		}
	})
}
