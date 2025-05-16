package codegen

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath" // Added for filepath.Dir
	"sync"
)

// WriteableFileSystem defines the necessary methods for a file system that can be written to.
// This allows for writing to different file system implementations, such as in-memory filesystems for testing.
type WriteableFileSystem interface {
	MkdirAll(path string, perm fs.FileMode) error
	WriteFile(filename string, data []byte, perm fs.FileMode) error
}

// File represents a file to be generated.
// Or an existing file provided as context.
type File struct {
	Path     string `json:"path"`
	Contents []byte `json:"contents"` // Changed to []byte to match os.WriteFile and API expectation for base64 encoded potentially
}

// DesiredFile represents a file the user wants to generate.
type DesiredFile struct {
	Path        string `json:"path"`
	Description string `json:"description"` // Human language description of desired contents
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

// GenerateCode concurrently generates multiple files based on an instruction, existing files, and desired output files.
func (g *Generator) GenerateCode(instruction string, existingFiles []File, desiredOutputFiles []DesiredFile) ([]File, error) {
	if len(desiredOutputFiles) == 0 {
		return []File{}, nil
	}

	generatedFiles := make([]File, len(desiredOutputFiles))
	// Using a channel to collect errors from goroutines.
	// A buffered channel matching the number of goroutines to prevent blocking.
	errs := make(chan error, len(desiredOutputFiles))
	var wg sync.WaitGroup

	for i, desiredFile := range desiredOutputFiles {
		wg.Add(1)
		go func(idx int, df DesiredFile) {
			defer wg.Done()
			// Placeholder for the actual call to the new private method
			// file, err := g.generateSingleFile(instruction, existingFiles, desiredOutputFiles, df)
			// For now, let's simulate a successful generation for structure
			// This will be replaced in the next step when generateSingleFile is implemented

			// Simulating a call to a method that will be implemented in a later step.
			// This method is expected to call makeChatCompletionsRequest (api.go)
			// and process its response to return a File object or an error.
			// file, err := g.generateSingleFile(instruction, existingFiles, desiredOutputFiles, df)

			// Temporarily, we pass only the necessary parts to makeAPIRequest for this step's structure,
			// knowing it will be replaced by g.generateSingleFile.
			// The makeAPIRequest will also be updated in a later step.
			// This is a structural placeholder.

			// Calling the new private method (to be fully implemented in the next step)
			file, err := g.generateSingleFile(instruction, existingFiles, desiredOutputFiles, df)
			if err != nil {
				errs <- fmt.Errorf("error generating file %s: %w", df.Path, err)
				return
			}
			generatedFiles[idx] = *file // Store a copy
		}(i, desiredFile)
	}

	wg.Wait()
	close(errs)

	// Check for errors
	// TODO: Aggregate multiple errors if necessary. For now, return the first one.
	for err := range errs {
		if err != nil {
			return nil, err // Return the first error encountered
		}
	}

	return generatedFiles, nil
}

// internal variable for testing purposes
var makeChatCompletionsRequestFunc = makeChatCompletionsRequest

// It constructs the necessary parameters and processes the API response.
func (g *Generator) generateSingleFile(instruction string, existingFiles []File, allDesiredFiles []DesiredFile, currentFileToGenerate DesiredFile) (*File, error) {
	// Call makeChatCompletionsRequest (from api.go) - this anticipates signature changes in api.go
	apiResp, err := makeChatCompletionsRequestFunc(g.apiKey, instruction, existingFiles, allDesiredFiles, currentFileToGenerate)
	if err != nil {
		return nil, fmt.Errorf("API request failed for %s: %w", currentFileToGenerate.Path, err)
	}

	if apiResp == nil {
		return nil, fmt.Errorf("received nil APIResponse for %s", currentFileToGenerate.Path)
	}

	// Extract content from APIResponse.Choices[0].Message.Content
	if len(apiResp.Choices) == 0 || apiResp.Choices[0].Message.Content == "" {
		// Check for API-level error in the response, even if HTTP status was 200
		if apiResp.Error != nil {
			return nil, fmt.Errorf("API returned an error for %s: %s (Type: %s, Code: %v)", currentFileToGenerate.Path, apiResp.Error.Message, apiResp.Error.Type, apiResp.Error.Code)
		}
		return nil, fmt.Errorf("API response for %s did not contain expected content. Choices: %d", currentFileToGenerate.Path, len(apiResp.Choices))
	}

	rawFileContentString := apiResp.Choices[0].Message.Content

	// Create and return codegen.File struct
	file := &File{
		Path:     currentFileToGenerate.Path,
		Contents: []byte(rawFileContentString),
	}

	return file, nil
}
