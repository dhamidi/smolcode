package codegen

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath" // Added for filepath.Dir
)

// WriteableFileSystem defines the necessary methods for a file system that can be written to.
// This allows for writing to different file system implementations, such as in-memory filesystems for testing.
type WriteableFileSystem interface {
	MkdirAll(path string, perm fs.FileMode) error
	WriteFile(filename string, data []byte, perm fs.FileMode) error
}

// File represents a file to be generated.
type File struct {
	Path     string `json:"path"`
	Contents []byte `json:"contents"` // Changed to []byte to match os.WriteFile and API expectation for base64 encoded potentially
}

// Generator is responsible for generating code.
type Generator struct {
	apiKey string
}

// New creates a new Generator.
func New(apiKey string) *Generator {
	return &Generator{apiKey: apiKey}
}

// Write writes the generated files to disk.
// It overwrites existing files.
func (g *Generator) Write(files []File) error {
	for _, file := range files {
		err := os.WriteFile(file.Path, file.Contents, 0644)
		if err != nil {
			return fmt.Errorf("error writing file %s: %w", file.Path, err)
		}
	}
	return nil
}

// WriteTo writes the generated files to the provided WriteableFileSystem.
// It creates necessary directories and overwrites existing files.
func (g *Generator) WriteTo(files []*File, destFS WriteableFileSystem) error {
	for _, file := range files {
		if file == nil {
			continue // Should not happen with well-formed input, but good for robustness
		}
		// Get the directory part of the path
		dir := filepath.Dir(file.Path)
		if dir != "" && dir != "." {
			// Create the directory structure in the destination FS
			// Using 0755 as a common permission for directories
			if err := destFS.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("error creating directory %s in destFS: %w", dir, err)
			}
		}

		// Write the file contents to the destination FS
		// Using 0644 as a common permission for files
		if err := destFS.WriteFile(file.Path, file.Contents, 0644); err != nil {
			return fmt.Errorf("error writing file %s to destFS: %w", file.Path, err)
		}
	}
	return nil
}

// GenerateCode sends the instruction and existing files to the API and returns the generated files.
func (g *Generator) GenerateCode(instruction string, existingFiles []File) ([]File, error) {
	// The makeAPIRequest function is expected to handle the conversion from API response to []File.
	// If existingFiles needs to be read from disk here, that logic would be added.
	// For now, assuming existingFiles are already populated with content if needed.
	generatedFiles, err := makeAPIRequest(g.apiKey, instruction, existingFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to generate code via API: %w", err)
	}
	return generatedFiles, nil
}
