package smolcode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"google.golang.org/genai"
)

var RecallMemoryTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "recall_memory",
				Description: strings.TrimSpace(
					`
Recalls facts from the knowledge base.

Either provide a specific 'factID' to retrieve a single fact, 
or provide an 'about' search term to find a relevant fact.

When searching, prefer to search with single words and narrow down as needed.
`,
				),
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"about": {
							Type:        genai.TypeString,
							Description: "A search term to find relevant facts in .smolcode/facts - this is searched verbatim",
						},
						"factID": {
							Type:        genai.TypeString,
							Description: "The specific ID of the fact to recall.",
						},
					},
					// Although technically optional, validation is done in the function
					Required: []string{}, // Neither is strictly required by schema, logic handles it
				},
			},
		},
	},
	Function: recallMemory,
}

func recallMemory(args map[string]any) (map[string]any, error) {
	factsDir := ".smolcode/facts"
	var about, factID string

	if aboutRaw, ok := args["about"]; ok {
		about, _ = aboutRaw.(string)
	}
	if factIDRaw, ok := args["factID"]; ok {
		factID, _ = factIDRaw.(string)
	}

	// Validation: factID takes precedence. At least one must be non-empty.
	if factID != "" {
		// Recall by specific ID
		if strings.Contains(factID, "/") || strings.Contains(factID, `\`) {
			return nil, fmt.Errorf("recall_memory: factID '%s' cannot contain slashes", factID)
		}
		filePath := filepath.Join(factsDir, factID+".md")
		content, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("recall_memory: fact with ID '%s' not found", factID)
			}
			return nil, fmt.Errorf("recall_memory: error reading fact '%s': %w", factID, err)
		}
		return map[string]any{
			"id":   factID,
			"fact": string(content),
		}, nil
	} else if about != "" {
		// Recall by search term using ripgrep (rg)
		cmd := exec.Command("rg", "--files-with-matches", "--fixed-strings", about, factsDir)
		output, err := cmd.Output()

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				// rg exits with 1 if no matches are found, 2 for other errors.
				if exitErr.ExitCode() == 1 {
					return nil, fmt.Errorf("recall_memory: no facts found matching '%s'", about)
				}
				// Include stderr for better debugging if available
				stderr := string(exitErr.Stderr)
				return nil, fmt.Errorf("recall_memory: rg command failed with exit code %d: %w. Stderr: %s", exitErr.ExitCode(), err, stderr)
			}
			// Other errors (e.g., rg not found)
			return nil, fmt.Errorf("recall_memory: failed to execute rg command: %w", err)
		}

		outputStr := strings.TrimSpace(string(output))
		if outputStr == "" { // Should be caught by exit code 1, but double check
			return nil, fmt.Errorf("recall_memory: no facts found matching '%s'", about)
		}

		// Process all matching files
		filePaths := strings.Split(outputStr, "\n")
		numMatches := len(filePaths)
		maxResults := 20

		var matches []map[string]string
		var remainingIDs []string

		for i, filePath := range filePaths {
			if filePath == "" { // Skip empty lines if any
				continue
			}
			// Clean up potential carriage returns from rg output on Windows
			filePath = strings.TrimSpace(filePath)
			if filePath == "" { // Skip if it was just whitespace
				continue
			}

			fileName := filepath.Base(filePath)
			extractedID := strings.TrimSuffix(fileName, ".md")

			if len(matches) < maxResults { // Check against actual matches found so far
				// Read the content of the matched file
				content, err := os.ReadFile(filePath) // Use the path rg returned
				if err != nil {
					// Check if file exists first before returning error, rg might return deleted file paths?
					if os.IsNotExist(err) {
						// File might have been deleted between rg finding it and us reading it. Skip it.
						// Optionally log this occurrence.
						fmt.Fprintf(os.Stderr, "recall_memory: warning: matched file disappeared before reading: %s\n", filePath)
						continue
					}
					// For other errors, return an error, as partial results might be confusing.
					return nil, fmt.Errorf("recall_memory: error reading matched fact file '%s': %w", filePath, err)
				}
				matches = append(matches, map[string]string{
					"id":   extractedID,
					"fact": string(content),
				})
			} else {
				remainingIDs = append(remainingIDs, extractedID)
			}
		}

		// Check if any valid matches were actually processed
		if len(matches) == 0 && len(remainingIDs) == 0 {
			// This could happen if all files found by rg disappeared before reading or rg returned only empty lines
			return nil, fmt.Errorf("recall_memory: no accessible facts found matching '%s'", about)
		}

		result := map[string]any{
			"matches": matches,
		}
		if len(remainingIDs) > 0 {
			result["remaining_ids"] = remainingIDs
		}

		return result, nil

	} else {
		// Neither factID nor about was provided
		return nil, fmt.Errorf("recall_memory: either 'factID' or 'about' parameter must be provided")
	}
}
