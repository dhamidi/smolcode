package codegen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerator_Write(t *testing.T) {
	gen := New("test-api-key")
	tempDir := t.TempDir()

	files := []File{
		{Path: filepath.Join(tempDir, "file1.txt"), Contents: []byte("hello")},
		{Path: filepath.Join(tempDir, "file2.txt"), Contents: []byte("world")},
	}

	// Test successful file creation
	err := gen.Write(files)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify file1.txt
	content, err := os.ReadFile(filepath.Join(tempDir, "file1.txt"))
	if err != nil {
		t.Fatalf("ReadFile failed for file1.txt: %v", err)
	}
	if string(content) != "hello" {
		t.Errorf("file1.txt content = %q, want %q", string(content), "hello")
	}

	// Verify file2.txt
	content, err = os.ReadFile(filepath.Join(tempDir, "file2.txt"))
	if err != nil {
		t.Fatalf("ReadFile failed for file2.txt: %v", err)
	}
	if string(content) != "world" {
		t.Errorf("file2.txt content = %q, want %q", string(content), "world")
	}

	// Test successful file overwriting
	overwriteFiles := []File{
		{Path: filepath.Join(tempDir, "file1.txt"), Contents: []byte("new hello")},
	}
	err = gen.Write(overwriteFiles)
	if err != nil {
		t.Fatalf("Write (overwrite) failed: %v", err)
	}

	content, err = os.ReadFile(filepath.Join(tempDir, "file1.txt"))
	if err != nil {
		t.Fatalf("ReadFile failed for overwritten file1.txt: %v", err)
	}
	if string(content) != "new hello" {
		t.Errorf("overwritten file1.txt content = %q, want %q", string(content), "new hello")
	}

	// Test error handling for write failures (e.g., invalid path)
	// Creating a directory where a file is supposed to be, to cause a write error.
	// Note: This might not work on all OSes or file systems as expected,
	// os.WriteFile might be able to write to some things that look like directories.
	// A more robust test might involve permissions, but that's harder to set up portably.
	errorFilePath := filepath.Join(tempDir, "error_dir")
	err = os.Mkdir(errorFilePath, 0755)
	if err != nil {
		t.Fatalf("Failed to create directory for error test: %v", err)
	}

	errorFiles := []File{
		{Path: errorFilePath, Contents: []byte("should fail")},
	}
	err = gen.Write(errorFiles)
	if err == nil {
		t.Errorf("Write to directory %s was expected to fail, but it did not", errorFilePath)
	}
}
