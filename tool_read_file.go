package smolcode

import (
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"unicode/utf8"

	"google.golang.org/genai"
)

var ReadFileTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "read_file",
				Description: "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"filepath": {
							Type:        genai.TypeString,
							Title:       "filepath",
							Description: "The relative path of a file in the working directory.",
						},
					},
				},
			},
		},
	},
	Function: func(args map[string]any) (map[string]any, error) {
		if args["filepath"] == nil {
			return nil, fmt.Errorf("read_file: no filepath provided")
		}
		providedPath := fmt.Sprintf("%s", args["filepath"])
		sanitizedFilename := path.Join(".", providedPath)
		contents, err := os.ReadFile(sanitizedFilename)
		if err != nil {
			return nil, fmt.Errorf("read_file: %w", err)
		}
		if utf8.Valid(contents) {
			return map[string]any{"contents": string(contents), "mime_type": "text/plain"}, nil
		} else {
			return map[string]any{"contents": base64.StdEncoding.EncodeToString(contents), "mime_type": "application/octet-stream"}, nil
		}
	},
}
