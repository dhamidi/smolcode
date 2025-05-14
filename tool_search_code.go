package smolcode

import (
	"fmt"
	"os/exec"
	"strings"

	"google.golang.org/genai"
)

var SearchCodeTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "search_code",
				Description: strings.TrimSpace(`
Search for exact text patterns in files using ripgrep, a fast keyword search tool.

WHEN TO USE THIS TOOL:
- When you need to find exact text matches like variable names, function calls, or specific strings
- When you know the precise pattern you're looking for (including regex patterns)
- When you want to quickly locate all occurrences of a specific term across multiple files
- When you need to search for code patterns with exact syntax

WHEN NOT TO USE THIS TOOL:
- For semantic or conceptual searches (e.g., "how does authentication work")
- For finding code that implements a certain functionality without knowing the exact terms
- When you already have read the entire file
`),
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"pattern": {
							Type:        genai.TypeString,
							Description: "The regexp pattern to search for.",
						},
						"directory": {
							Type:        genai.TypeString,
							Description: "Optional directory to scope the search.",
						},
					},
					Required: []string{"pattern"},
				},
			},
		},
	},
	Function: searchCode,
}

func searchCode(args map[string]any) (map[string]any, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return nil, fmt.Errorf("search_code: pattern is required and must be a non-empty string")
	}

	directory, _ := args["directory"].(string) // directory is optional

	cmdArgs := []string{"rg", "--json", pattern} // Added --json for structured output
	if directory != "" {
		cmdArgs = append(cmdArgs, directory)
	}

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	if output, err := cmd.CombinedOutput(); err != nil {
		// rg exits with 1 if no matches are found, which is not an error for us.
		// It exits with 2 for actual errors.
		exitErr, ok := err.(*exec.ExitError)
		if ok && exitErr.ExitCode() == 1 {
			// No matches found, return empty results
			return map[string]any{"results": "[]"}, nil // Return empty JSON array string
		}
		// Otherwise, it's a real error
		return nil, fmt.Errorf("search_code: failed to run command '%s': %w (output: %s)", strings.Join(cmdArgs, " "), err, output)
	} else {
		// Combine multiple JSON objects into a single JSON array string
		outputStr := strings.TrimSpace(string(output))
		lines := strings.Split(outputStr, "\n")
		jsonArray := "[" + strings.Join(lines, ",") + "]"

		return map[string]any{"results": jsonArray}, nil
	}
}
