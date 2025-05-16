package codegen

import (
	"bytes" // For comparing byte slices
	"io/fs"
	"os" // Added for os.IsNotExist
	"testing"

	"github.com/spf13/afero"
	// Assuming your module path is "project", replace if different
	// For example: "github.com/username/project/codegen"
	// For this tool, we'll assume codegen is in the same package scope for testing.
)

// TestFsAdapter adapts afero.Fs to codegen.WriteableFileSystem for testing purposes.
type TestFsAdapter struct {
	afero.Fs
}

// WriteFile implements the WriteableFileSystem interface for TestFsAdapter.
func (a *TestFsAdapter) WriteFile(filename string, data []byte, perm fs.FileMode) error {
	return afero.WriteFile(a.Fs, filename, data, perm)
}

// MkdirAll on TestFsAdapter uses the embedded afero.Fs's MkdirAll, which matches the interface.

func TestGenerator_WriteTo(t *testing.T) {
	gen := New("") // API key not needed for this test

	// Use afero.NewMemMapFs for an in-memory file system
	memFS := afero.NewMemMapFs()
	adapter := &TestFsAdapter{Fs: memFS}

	filesToTest := []*File{
		{Path: "file1.txt", Contents: []byte("hello world")},
		{Path: "dira/file2.txt", Contents: []byte("contents of file2")},
		{Path: "dira/dirb/file3.txt", Contents: []byte("nested file")},
		{Path: "nodirfile.md", Contents: []byte("# Markdown")},
	}

	err := gen.WriteTo(filesToTest, adapter)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	// Verify files
	for _, f := range filesToTest {
		data, err := afero.ReadFile(memFS, f.Path)
		if err != nil {
			t.Errorf("ReadFile failed for %s: %v", f.Path, err)
			continue
		}
		if !bytes.Equal(data, f.Contents) {
			t.Errorf("Contents incorrect for %s: got %q, want %q", f.Path, string(data), string(f.Contents))
		}

		// Optionally, check permissions if important and easily verifiable with afero
		// info, err := memFS.Stat(f.Path)
		// if err != nil {
		// 	t.Errorf("Stat failed for %s: %v", f.Path, err)
		// 	continue
		// }
		// if info.Mode().Perm() != 0644 { // Example permission check
		// 	t.Errorf("Permissions incorrect for %s: got %v, want %v", f.Path, info.Mode().Perm(), fs.FileMode(0644))
		// }
	}

	// Verify directory creation (optional, as file writing implies dir creation by MkdirAll)
	// Check if "dira" and "dira/dirb" exist and are directories
	dirInfoA, err := memFS.Stat("dira")
	if err != nil {
		t.Fatalf("Stat failed for directory dira: %v", err)
	}
	if !dirInfoA.IsDir() {
		t.Errorf("dira is not a directory")
	}

	dirInfoB, err := memFS.Stat("dira/dirb")
	if err != nil {
		t.Fatalf("Stat failed for directory dira/dirb: %v", err)
	}
	if !dirInfoB.IsDir() {
		t.Errorf("dira/dirb is not a directory")
	}
}

func TestGenerator_WriteTo_NilFileInSlice(t *testing.T) {
	gen := New("")
	memFS := afero.NewMemMapFs()
	adapter := &TestFsAdapter{Fs: memFS}

	filesToTest := []*File{
		{Path: "file1.txt", Contents: []byte("hello")},
		nil, // Test robustness with nil file
		{Path: "file2.txt", Contents: []byte("world")},
	}

	err := gen.WriteTo(filesToTest, adapter)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	// Check file1.txt
	data, err := afero.ReadFile(memFS, "file1.txt")
	if err != nil {
		t.Errorf("ReadFile failed for file1.txt: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("Contents for file1.txt: got %q, want %q", string(data), "hello")
	}

	// Check file2.txt
	data2, err2 := afero.ReadFile(memFS, "file2.txt")
	if err2 != nil {
		t.Errorf("ReadFile failed for file2.txt: %v", err2)
	}
	if string(data2) != "world" {
		t.Errorf("Contents for file2.txt: got %q, want %q", string(data2), "world")
	}
}

func TestGenerator_WriteTo_EmptyFilesSlice(t *testing.T) {
	gen := New("")
	memFS := afero.NewMemMapFs()
	adapter := &TestFsAdapter{Fs: memFS}

	var filesToTest []*File // Empty slice

	err := gen.WriteTo(filesToTest, adapter)
	if err != nil {
		t.Fatalf("WriteTo failed for empty slice: %v", err)
	}
	// No files should be written, memFS should be empty
	// We can check by trying to list files or checking a known non-existent file
	_, err = memFS.Stat("anyfile.txt")
	if !os.IsNotExist(err) { // Using os.IsNotExist to check for file not existing
		t.Errorf("Expected file not to exist, but Stat gave: %v", err)
	}
}
