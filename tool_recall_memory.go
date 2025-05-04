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
		words := strings.Fields(about)
		if len(words) == 0 {
			return nil, fmt.Errorf("recall_memory: 'about' parameter cannot be empty or only whitespace")
		}

		// Use a map to store file paths found for the *first* word,
		// and then use it to track the intersection for subsequent words.
		// map[string]struct{} acts like a set.
		intersectingFiles := make(map[string]struct{})

		for i, word := range words {
			cmd := exec.Command("rg", "--files-with-matches", "--smart-case", "--fixed-strings", word, factsDir)
			output, err := cmd.Output()

			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					if exitErr.ExitCode() == 1 {
						// If any word is not found, the intersection is empty.
						return nil, fmt.Errorf("recall_memory: no facts found containing the word '%s' (and thus not all words in '%s')", word, about)
					}
					stderr := string(exitErr.Stderr)
					return nil, fmt.Errorf("recall_memory: rg command failed for word '%s': %w. Stderr: %s", word, err, stderr)
				}
				return nil, fmt.Errorf("recall_memory: failed to execute rg command for word '%s': %w", word, err)
			}

			outputStr := strings.TrimSpace(string(output))
			if outputStr == "" { // Should be caught by exit code 1, but double check
				return nil, fmt.Errorf("recall_memory: no facts found containing the word '%s' (and thus not all words in '%s')", word, about)
			}

			currentWordFiles := make(map[string]struct{})
			filePaths := strings.Split(outputStr, "\n")
			for _, filePath := range filePaths {
				cleanedPath := strings.TrimSpace(filePath)
				if cleanedPath != "" {
					currentWordFiles[cleanedPath] = struct{}{}
				}
			}

			if i == 0 {
				// For the first word, initialize the intersection set
				intersectingFiles = currentWordFiles
			} else {
				// For subsequent words, compute the intersection
				nextIntersection := make(map[string]struct{})
				for path := range intersectingFiles {
					if _, exists := currentWordFiles[path]; exists {
						nextIntersection[path] = struct{}{}
					}
				}
				intersectingFiles = nextIntersection
			}

			// If intersection becomes empty, no need to check further words
			if len(intersectingFiles) == 0 {
				return nil, fmt.Errorf("recall_memory: no facts found containing all words in '%s'", about)
			}
		}

		// Process all files in the final intersection
		maxResults := 20
		var matches []map[string]string
		var remainingIDs []string
		processedCount := 0 // Keep track of how many we add to matches

		for filePath := range intersectingFiles {
			if filePath == "" { // Should not happen after cleaning, but safeguard
				continue
			}

			fileName := filepath.Base(filePath)
			extractedID := strings.TrimSuffix(fileName, ".md")

			if processedCount < maxResults {
				content, err := os.ReadFile(filePath) // Use the path from the intersection map
				if err != nil {
					if os.IsNotExist(err) {
						fmt.Fprintf(os.Stderr, "recall_memory: warning: intersecting file disappeared before reading: %s\n", filePath)
						continue // Skip this file
					}
					return nil, fmt.Errorf("recall_memory: error reading intersecting fact file '%s': %w", filePath, err)
				}
				matches = append(matches, map[string]string{
					"id":   extractedID,
					"fact": string(content),
				})
				processedCount++
			} else {
				remainingIDs = append(remainingIDs, extractedID)
			}
		}

		// Check if any valid matches were actually processed after intersection
		if len(matches) == 0 {
			// This can happen if files disappear or if the intersection was truly empty
			// (though the earlier check should catch empty intersection).
			return nil, fmt.Errorf("recall_memory: no accessible facts found containing all words in '%s'", about)
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
