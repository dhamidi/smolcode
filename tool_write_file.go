package smolcode

import (
	"fmt"
	"os"
	"path"
	"strings"

	"google.golang.org/genai"
)

var WriteFileTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "write_file",
				Description: strings.TrimSpace(
					`
Overwrites a file with new content.

If the file specified with path doesn't exist, it will be created.
`),
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"filepath": {
							Type:        genai.TypeString,
							Title:       "filepath",
							Description: "The file to write – must be a relative path of a file in the working directory.",
						},
						"content": {
							Type:        genai.TypeString,
							Title:       "content",
							Description: "The new content to write to the file.",
						},
					},
					Required: []string{"filepath", "content"},
				},
			},
		},
	},
	Function: writeFile,
}

func writeFile(args map[string]any) (map[string]any, error) {
	filepath := fmt.Sprintf("%s", args["filepath"])
	if filepath == "" {
		return nil, fmt.Errorf("write_file: filepath is missing")
	}

	content := fmt.Sprintf("%s", args["content"])

	// Create directory if it doesn't exist
	dir := path.Dir(filepath)
	if dir != "." {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return nil, fmt.Errorf("write_file: failed to create directory: %w", err)
		}
	}

	err := os.WriteFile(filepath, []byte(content), 0644)
	if err != nil {
		return nil, fmt.Errorf("write_file: failed to write file: %w", err)
	}

	return map[string]any{"wrote": filepath}, nil
}
