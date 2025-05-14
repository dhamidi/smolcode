package main

import (
	"fmt"
	"log"
	"os"

	"github.com/dhamidi/smolcode/codegen"
	"google.golang.org/genai"
)

var CodegenTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "perform_code_generation",
				Description: "Generates code based on an instruction and optional existing files, then writes the generated files to disk. Uses the Inceptionlabs API.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"instruction": {
							Type:        genai.TypeString,
							Description: "The detailed instruction or prompt for what code to generate.",
						},
						"existing_files": {
							Type:        genai.TypeArray,
							Description: "Optional. An array of existing files that provide context. Each item should be an object with 'path' (string) and 'contents' (string, base64 encoded for binary).",
							Items: &genai.Schema{
								Type: genai.TypeObject,
								Properties: map[string]*genai.Schema{
									"path":     {Type: genai.TypeString},
									"contents": {Type: genai.TypeString}, // Assuming agent sends string, will be decoded if base64
								},
								Required: []string{"path", "contents"},
							},
						},
					},
					Required: []string{"instruction"},
				},
			},
		},
	},
	Function: func(args map[string]any) (map[string]any, error) {
		instruction, ok := args["instruction"].(string)
		if !ok || instruction == "" {
			return nil, fmt.Errorf("perform_code_generation: missing or invalid 'instruction' parameter")
		}

		var existingCodegenFiles []codegen.File
		if argFiles, ok := args["existing_files"].([]interface{}); ok {
			for i, fileArg := range argFiles {
				if fileMap, ok := fileArg.(map[string]interface{}); ok {
					path, pathOk := fileMap["path"].(string)
					contentsStr, contentsOk := fileMap["contents"].(string)
					if !pathOk || !contentsOk {
						return nil, fmt.Errorf("perform_code_generation: existing_files item %d is invalid: missing path or contents, or wrong type", i)
					}
					// Assuming contents are plain text or base64 string that codegen.File can handle or be adapted for.
					// For now, directly converting string to []byte. If base64 is used by agent, decoding needed here.
					existingCodegenFiles = append(existingCodegenFiles, codegen.File{Path: path, Contents: []byte(contentsStr)})
				} else {
					return nil, fmt.Errorf("perform_code_generation: existing_files item %d is not a valid object", i)
				}
			}
		}

		apiKey := os.Getenv("INCEPTION_API_KEY")
		if apiKey == "" {
			// Log this, but the codegen package itself might also return an error if key is empty.
			// For a tool, it might be better to return an error immediately.
			log.Println("Warning: INCEPTION_API_KEY environment variable not set. Code generation will likely fail.")
			// Depending on how strictly tools should handle this, could return error here:
			// return nil, fmt.Errorf("perform_code_generation: INCEPTION_API_KEY not set")
		}

		generator := codegen.New(apiKey)
		generatedFiles, err := generator.GenerateCode(instruction, existingCodegenFiles)
		if err != nil {
			return nil, fmt.Errorf("perform_code_generation: error from GenerateCode: %w", err)
		}

		if len(generatedFiles) == 0 {
			return map[string]any{"result": "Code generation completed, but no files were returned by the API."}, nil
		}

		err = generator.Write(generatedFiles)
		if err != nil {
			return nil, fmt.Errorf("perform_code_generation: error from Write: %w", err)
		}

		writtenPaths := make([]string, len(generatedFiles))
		for i, f := range generatedFiles {
			writtenPaths[i] = f.Path
		}

		return map[string]any{
			"result":        fmt.Sprintf("Successfully generated and wrote %d file(s).", len(generatedFiles)),
			"files_written": writtenPaths,
		}, nil
	},
}
