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

// --- isWorkspaceRoot --------------------------------------------------------

func TestIsWorkspaceRoot(t *testing.T) {
	cases := []struct {
		name          string
		path          string
		workspaceRoot string
		want          bool
	}{
		{
			name:          "exact match",
			path:          "/home/user/project",
			workspaceRoot: "/home/user/project",
			want:          true,
		},
		{
			name:          "path has trailing slash",
			path:          "/home/user/project/",
			workspaceRoot: "/home/user/project",
			want:          true,
		},
		{
			name:          "workspaceRoot has trailing slash",
			path:          "/home/user/project",
			workspaceRoot: "/home/user/project/",
			want:          true,
		},
		{
			name:          "relative variant cleans to the same path",
			path:          "/home/user/project/../project",
			workspaceRoot: "/home/user/project",
			want:          true,
		},
		{
			name:          "different path",
			path:          "/home/user/project/subdir",
			workspaceRoot: "/home/user/project",
			want:          false,
		},
		{
			name:          "empty path",
			path:          "",
			workspaceRoot: "/home/user/project",
			want:          false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isWorkspaceRoot(c.path, c.workspaceRoot)
			if got != c.want {
				t.Errorf("isWorkspaceRoot(%q, %q) = %v, want %v", c.path, c.workspaceRoot, got, c.want)
			}
		})
	}
}

// --- DeleteEntry -------------------------------------------------------------

func TestDeleteEntry(t *testing.T) {
	t.Run("deletes an existing file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(filePath, []byte("hi"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		app := &App{}
		if err := app.DeleteEntry(filePath, dir); err != nil {
			t.Fatalf("DeleteEntry: unexpected error: %v", err)
		}
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Errorf("expected %q to no longer exist, got err=%v", filePath, err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("expected parent dir empty, got %d entries", len(entries))
		}
	})

	t.Run("deletes a folder with nested files and subfolders", func(t *testing.T) {
		dir := t.TempDir()
		folder := filepath.Join(dir, "folder")
		nested := filepath.Join(folder, "nested")
		if err := os.MkdirAll(nested, 0755); err != nil {
			t.Fatalf("mkdir nested: %v", err)
		}
		nestedFile := filepath.Join(nested, "inner.txt")
		if err := os.WriteFile(nestedFile, []byte("data"), 0644); err != nil {
			t.Fatalf("write nested file: %v", err)
		}

		app := &App{}
		if err := app.DeleteEntry(folder, dir); err != nil {
			t.Fatalf("DeleteEntry: unexpected error: %v", err)
		}
		if _, err := os.Stat(folder); !os.IsNotExist(err) {
			t.Errorf("expected %q to no longer exist, got err=%v", folder, err)
		}
	})

	t.Run("errors deleting a path removed externally", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(dir, "does-not-exist.txt")

		app := &App{}
		err := app.DeleteEntry(missing, dir)
		if err == nil {
			t.Fatal("expected error deleting a nonexistent path, got nil")
		}
	})

	t.Run("rejects deleting the workspace root", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("hi"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		app := &App{}
		err := app.DeleteEntry(dir, dir)
		if err == nil {
			t.Fatal("expected error deleting the workspace root, got nil")
		}
		if _, statErr := os.Stat(dir); statErr != nil {
			t.Errorf("expected workspace root to still exist, got err=%v", statErr)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "keep.txt")); statErr != nil {
			t.Errorf("expected workspace root contents untouched, got err=%v", statErr)
		}
	})
}

// --- RenameEntry ---------------------------------------------------------

func TestRenameEntry(t *testing.T) {
	t.Run("renames an existing file with a valid, non-colliding name", func(t *testing.T) {
		dir := t.TempDir()
		oldPath := filepath.Join(dir, "old.txt")
		if err := os.WriteFile(oldPath, []byte("hi"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		app := &App{}
		newPath, err := app.RenameEntry(oldPath, "new.txt", dir)
		if err != nil {
			t.Fatalf("RenameEntry: unexpected error: %v", err)
		}
		wantPath := filepath.Join(dir, "new.txt")
		if newPath != wantPath {
			t.Errorf("RenameEntry returned %q, want %q", newPath, wantPath)
		}
		if _, err := os.Stat(wantPath); err != nil {
			t.Errorf("expected renamed file at %q: %v", wantPath, err)
		}
		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Errorf("expected old path %q to no longer exist, got err=%v", oldPath, err)
		}
	})

	t.Run("renames a folder, reusing moveOrRename", func(t *testing.T) {
		dir := t.TempDir()
		oldPath := filepath.Join(dir, "oldfolder")
		nested := filepath.Join(oldPath, "nested")
		if err := os.MkdirAll(nested, 0755); err != nil {
			t.Fatalf("mkdir nested: %v", err)
		}
		nestedFile := filepath.Join(nested, "inner.txt")
		if err := os.WriteFile(nestedFile, []byte("data"), 0644); err != nil {
			t.Fatalf("write nested file: %v", err)
		}

		app := &App{}
		newPath, err := app.RenameEntry(oldPath, "newfolder", dir)
		if err != nil {
			t.Fatalf("RenameEntry: unexpected error: %v", err)
		}
		wantPath := filepath.Join(dir, "newfolder")
		if newPath != wantPath {
			t.Errorf("RenameEntry returned %q, want %q", newPath, wantPath)
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

	t.Run("errors renaming a path removed externally", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(dir, "does-not-exist.txt")

		app := &App{}
		_, err := app.RenameEntry(missing, "new.txt", dir)
		if err == nil {
			t.Fatal("expected error renaming a nonexistent path, got nil")
		}
	})

	t.Run("rejects renaming the workspace root", func(t *testing.T) {
		dir := t.TempDir()

		app := &App{}
		_, err := app.RenameEntry(dir, "newname", dir)
		if err == nil {
			t.Fatal("expected error renaming the workspace root, got nil")
		}
		if _, statErr := os.Stat(dir); statErr != nil {
			t.Errorf("expected workspace root to still exist, got err=%v", statErr)
		}
	})

	t.Run("rejects renaming to the same name", func(t *testing.T) {
		dir := t.TempDir()
		oldPath := filepath.Join(dir, "foo.txt")
		if err := os.WriteFile(oldPath, []byte("hi"), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		app := &App{}
		_, err := app.RenameEntry(oldPath, "foo.txt", dir)
		if err == nil {
			t.Fatal("expected error renaming to the same name, got nil")
		}
		wantMsg := "already named that"
		if err.Error() != wantMsg {
			t.Errorf("error = %q, want %q", err.Error(), wantMsg)
		}
		if _, statErr := os.Stat(oldPath); statErr != nil {
			t.Errorf("expected original file untouched, got err=%v", statErr)
		}
	})

	t.Run("rejects a name collision with an existing sibling", func(t *testing.T) {
		dir := t.TempDir()
		oldPath := filepath.Join(dir, "foo.txt")
		if err := os.WriteFile(oldPath, []byte("hi"), 0644); err != nil {
			t.Fatalf("write foo: %v", err)
		}
		siblingPath := filepath.Join(dir, "bar.txt")
		if err := os.WriteFile(siblingPath, []byte("hi"), 0644); err != nil {
			t.Fatalf("write bar: %v", err)
		}

		app := &App{}
		_, err := app.RenameEntry(oldPath, "bar.txt", dir)
		if err == nil {
			t.Fatal("expected error on name collision, got nil")
		}
		if _, statErr := os.Stat(oldPath); statErr != nil {
			t.Errorf("expected original file untouched, got err=%v", statErr)
		}
	})

	t.Run("rejects invalid names", func(t *testing.T) {
		invalidNames := []string{"", ".", "..", "has/slash", `has\backslash`}
		for _, name := range invalidNames {
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				oldPath := filepath.Join(dir, "foo.txt")
				if err := os.WriteFile(oldPath, []byte("hi"), 0644); err != nil {
					t.Fatalf("write foo: %v", err)
				}

				app := &App{}
				_, err := app.RenameEntry(oldPath, name, dir)
				if err == nil {
					t.Fatalf("expected error renaming to invalid name %q, got nil", name)
				}
				if _, statErr := os.Stat(oldPath); statErr != nil {
					t.Errorf("expected original file untouched, got err=%v", statErr)
				}
			})
		}
	})
}

// --- CreateFile ------------------------------------------------------------

func TestCreateFile(t *testing.T) {
	t.Run("creates a file successfully", func(t *testing.T) {
		dir := t.TempDir()

		app := &App{}
		newPath, err := app.CreateFile(dir, "new.txt")
		if err != nil {
			t.Fatalf("CreateFile: unexpected error: %v", err)
		}
		wantPath := filepath.Join(dir, "new.txt")
		if newPath != wantPath {
			t.Errorf("CreateFile returned %q, want %q", newPath, wantPath)
		}
		if _, statErr := os.Stat(wantPath); statErr != nil {
			t.Errorf("expected created file at %q: %v", wantPath, statErr)
		}
	})

	t.Run("rejects a name collision with an existing file", func(t *testing.T) {
		dir := t.TempDir()
		existing := filepath.Join(dir, "foo.txt")
		if err := os.WriteFile(existing, []byte("hi"), 0644); err != nil {
			t.Fatalf("write foo: %v", err)
		}

		app := &App{}
		_, err := app.CreateFile(dir, "foo.txt")
		if err == nil {
			t.Fatal("expected error on name collision, got nil")
		}
		wantMsg := "foo.txt already exists"
		if err.Error() != wantMsg {
			t.Errorf("error = %q, want %q", err.Error(), wantMsg)
		}
	})
}

// --- CreateFolder ------------------------------------------------------------

func TestCreateFolder(t *testing.T) {
	t.Run("creates a folder successfully", func(t *testing.T) {
		dir := t.TempDir()

		app := &App{}
		newPath, err := app.CreateFolder(dir, "newfolder")
		if err != nil {
			t.Fatalf("CreateFolder: unexpected error: %v", err)
		}
		wantPath := filepath.Join(dir, "newfolder")
		if newPath != wantPath {
			t.Errorf("CreateFolder returned %q, want %q", newPath, wantPath)
		}
		info, statErr := os.Stat(wantPath)
		if statErr != nil {
			t.Fatalf("expected created folder at %q: %v", wantPath, statErr)
		}
		if !info.IsDir() {
			t.Errorf("expected %q to be a folder", wantPath)
		}
	})

	t.Run("rejects a name collision with an existing folder", func(t *testing.T) {
		dir := t.TempDir()
		existing := filepath.Join(dir, "existingfolder")
		if err := os.Mkdir(existing, 0755); err != nil {
			t.Fatalf("mkdir existingfolder: %v", err)
		}

		app := &App{}
		_, err := app.CreateFolder(dir, "existingfolder")
		if err == nil {
			t.Fatal("expected error on name collision, got nil")
		}
		wantMsg := "existingfolder already exists"
		if err.Error() != wantMsg {
			t.Errorf("error = %q, want %q", err.Error(), wantMsg)
		}
	})
}
