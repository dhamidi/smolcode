package main

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

type TarballWriterFS struct {
	tw          *tar.Writer
	createdDirs map[string]bool
}

func NewTarballWriterFS(target io.Writer) *TarballWriterFS {
	return &TarballWriterFS{
		tw:          tar.NewWriter(target),
		createdDirs: make(map[string]bool),
	}
}

// MkdirAll creates directory entries in the tar archive.
func (tfs *TarballWriterFS) MkdirAll(path string, perm fs.FileMode) error {
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == "" {
		return nil
	}
	dirPath := strings.TrimSuffix(cleanPath, "/") + "/" // Ensure it's a directory path

	// Iterate through parent components to ensure they are all created if not already present
	currentProcessingPath := ""
	parts := strings.Split(strings.Trim(dirPath, "/"), "/")

	for _, part := range parts {
		if part == "" {
			continue
		}
		currentProcessingPath = filepath.Join(currentProcessingPath, part)
		entryPath := strings.TrimSuffix(currentProcessingPath, "/") + "/"

		if !tfs.createdDirs[entryPath] {
			hdr := &tar.Header{
				Name:     entryPath,
				Mode:     int64(perm | fs.ModeDir), // Ensure directory bit is set, use provided perm
				Typeflag: tar.TypeDir,
				ModTime:  time.Now(),
			}
			if err := tfs.tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("tarball: failed to write header for directory %s: %w", entryPath, err)
			}
			tfs.createdDirs[entryPath] = true
		}
	}
	return nil
}

// WriteFile writes a file to the tar archive.
func (tfs *TarballWriterFS) WriteFile(filename string, data []byte, perm fs.FileMode) error {
	cleanFilename := filepath.Clean(filename)
	hdr := &tar.Header{
		Name:     cleanFilename,
		Size:     int64(len(data)),
		Mode:     int64(perm &^ fs.ModeDir), // Ensure it's a regular file mode
		ModTime:  time.Now(),
		Typeflag: tar.TypeReg,
	}

	if err := tfs.tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("tarball: failed to write header for file %s: %w", cleanFilename, err)
	}
	if _, err := tfs.tw.Write(data); err != nil {
		return fmt.Errorf("tarball: failed to write contents for file %s: %w", cleanFilename, err)
	}
	return nil
}

// Close finishes the tar archive. This must be called.
func (tfs *TarballWriterFS) Close() error {
	return tfs.tw.Close()
}
