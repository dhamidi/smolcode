package smolcode

import (
	"fmt"
	"os"
	"path/filepath" // Use filepath for OS-independent path joining
	"strings"

	"google.golang.org/genai"
)

var CreateMemoryTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "create_memory",
				Description: strings.TrimSpace(
					`
Stores facts in the knowledge base.

Each fact is written to '.smolcode/facts/<fact-id>.md'. Existing facts are overwritten.
`,
				),
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"facts": {
							Type:        genai.TypeArray,
							Description: "List of fact objects to store.",
							Items: &genai.Schema{
								Type: genai.TypeObject,
								Properties: map[string]*genai.Schema{
									"id": {
										Type:        genai.TypeString,
										Description: "A short identifier for the fact (e.g., 'project-language').",
									},
									"fact": {
										Type:        genai.TypeString,
										Description: "The fact itself (2-3 sentences max).",
									},
								},
								Required: []string{"id", "fact"},
							},
						},
					},
					Required: []string{"facts"},
				},
			},
		},
	},
	Function: createMemory,
}

// Fact represents a single piece of information to be stored.
type Fact struct {
	ID   string `json:"id"`
	Fact string `json:"fact"`
}

func createMemory(args map[string]any) (map[string]any, error) {
	factsRaw, ok := args["facts"]
	if !ok {
		return nil, fmt.Errorf("create_memory: missing required parameter 'facts'")
	}

	factsList, ok := factsRaw.([]interface{}) // The genai library decodes JSON arrays as []interface{}
	if !ok {
		// Attempt to handle the case where it might be decoded as []map[string]interface{} directly
		// This depends on the JSON decoder used internally by the genai library/caller
		altList, altOk := factsRaw.([]map[string]interface{})
		if !altOk {
			return nil, fmt.Errorf("create_memory: 'facts' parameter is not a valid list (got %T)", factsRaw)
		}
		// Convert []map[string]interface{} to []interface{} for uniform processing
		factsList = make([]interface{}, len(altList))
		for i, v := range altList {
			factsList[i] = v
		}
	}

	addedFacts := []string{}
	updatedFacts := []string{}
	factsDir := ".smolcode/facts"

	// Ensure the base directory exists
	err := os.MkdirAll(factsDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("create_memory: failed to create directory %s: %w", factsDir, err)
	}

	for i, factItem := range factsList {
		factMap, ok := factItem.(map[string]interface{}) // Each item in the list should be a map
		if !ok {
			return nil, fmt.Errorf("create_memory: fact item at index %d is not a valid object (got %T)", i, factItem)
		}

		idRaw, idOk := factMap["id"]
		factRaw, factOk := factMap["fact"]

		if !idOk || !factOk {
			return nil, fmt.Errorf("create_memory: fact item at index %d is missing 'id' or 'fact'", i)
		}

		idStr, idStrOk := idRaw.(string)
		factStr, factStrOk := factRaw.(string)

		if !idStrOk || !factStrOk {
			return nil, fmt.Errorf("create_memory: fact item at index %d has non-string 'id' or 'fact'", i)
		}

		if idStr == "" {
			return nil, fmt.Errorf("create_memory: fact item at index %d has an empty 'id'", i)
		}
		if strings.Contains(idStr, "/") || strings.Contains(idStr, `\`) { // Basic check for invalid path chars
			return nil, fmt.Errorf("create_memory: fact item 'id' ('%s') cannot contain slashes", idStr)
		}

		// Construct file path using filepath.Join for OS compatibility
		filePath := filepath.Join(factsDir, idStr+".md")

		// Check if file exists to determine added vs. updated
		_, err := os.Stat(filePath)
		fileExists := !os.IsNotExist(err) // If err is nil or *not* IsNotExist, the file exists.

		// Write the fact content to the file
		err = os.WriteFile(filePath, []byte(factStr), 0644)
		if err != nil {
			return nil, fmt.Errorf("create_memory: failed to write fact '%s' to %s: %w", idStr, filePath, err)
		}

		if fileExists {
			updatedFacts = append(updatedFacts, idStr)
		} else {
			addedFacts = append(addedFacts, idStr)
		}
	}

	response := map[string]any{
		"added":   addedFacts,
		"updated": updatedFacts,
	}

	return response, nil
}
