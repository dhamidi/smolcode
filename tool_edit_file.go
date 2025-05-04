package smolcode

import (
	"fmt"
	"os"
	"path"
	"strings"

	"google.golang.org/genai"
)

var EditFileTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "edit_file",
				Description: strings.TrimSpace(
					`
Make edits to a text file.

Replaces 'old_str' with 'new_str' in the given file. 'old_str' and 'new_str' MUST be different from each other.

If the file specified with path doesn't exist, it will be created.
`),
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"filepath": {
							Type:        genai.TypeString,
							Title:       "filepath",
							Description: "The file to edit – must be a relative path of a file in the working directory.",
						},
						"old_str": {
							Type:        genai.TypeString,
							Title:       "old_str",
							Description: "Text to search for - must match exactly and must only have one match exactly",
						},
						"new_str": {
							Type:        genai.TypeString,
							Title:       "new_str",
							Description: "Text to replace old_str with",
						},
					},
					Required: []string{"filepath", "old_str", "new_str"},
				},
			},
		},
	},
	Function: editFile,
}

func editFile(args map[string]any) (map[string]any, error) {
	filepath := fmt.Sprintf("%s", args["filepath"])
	if filepath == "" {
		return nil, fmt.Errorf("edit_file: filepath is missing")
	}

	oldStr := fmt.Sprintf("%s", args["old_str"])
	newStr := fmt.Sprintf("%s", args["new_str"])

	if oldStr == newStr {
		return nil, fmt.Errorf("edit_file: old_str and new_str must be different")
	}

	content, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) && oldStr == "" {
			return createNewFile(filepath, newStr)
		}
		return nil, err
	}

	oldContent := string(content)
	newContent := strings.Replace(oldContent, oldStr, newStr, -1)

	if oldContent == newContent && oldStr != "" {
		return nil, fmt.Errorf("edit_file: old_str not found in file")
	}

	err = os.WriteFile(filepath, []byte(newContent), 0644)
	if err != nil {
		return nil, err
	}

	return map[string]any{"wrote": filepath}, nil
}

func createNewFile(filePath, content string) (map[string]any, error) {
	dir := path.Dir(filePath)
	if dir != "." {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return nil, fmt.Errorf("edit_file: failed to create directory: %w", err)
		}
	}

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return nil, fmt.Errorf("edit_file: failed to create file: %w", err)
	}

	return map[string]any{"created": filePath}, nil
}
