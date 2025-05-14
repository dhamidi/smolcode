package codegen

import (
	"fmt"
	"os"
)

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
