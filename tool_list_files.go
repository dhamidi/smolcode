package smolcode

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"google.golang.org/genai"
)

var ListFilesTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "list_files",
				Description: "List files and directories at a given path. If no path is provided, lists files in the current directory.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"filepath": {
							Type:        genai.TypeString,
							Title:       "filepath",
							Description: "The relative path of a directory in the working directory.",
						},
					},
				},
			},
		},
	},
	Function: func(args map[string]any) (map[string]any, error) {
		providedPath := "."
		if args["filepath"] != nil {
			providedPath = fmt.Sprintf("%s", args["filepath"])
		}
		dir := filepath.Join(".", providedPath)
		files := []string{}
		err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}

			if relPath != "." && relPath != ".git" {
				if info.IsDir() {
					files = append(files, relPath+"/")
				} else {
					files = append(files, relPath)
				}
			}

			return nil
		})

		if err != nil {
			return nil, err
		}

		return map[string]any{"files": files}, nil
	},
}
