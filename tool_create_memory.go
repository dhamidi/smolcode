package smolcode

import (
	"fmt"
	// "os" // No longer directly needed for file operations
	// "path/filepath" // No longer directly needed
	"strings"

	"github.com/dhamidi/smolcode/memory"
	"google.golang.org/genai"
)

const memoryDBPath = ".smolcode/memory.db"

var CreateMemoryTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "create_memory",
				Description: strings.TrimSpace(
					`
Stores facts in the knowledge base.

Each fact is written to '.smolcode/facts/<fact-id>.md'. Existing facts are overwritten.

Use this when you are asked to memorize or remember something.

You are responsible for generating fact IDs.
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
		altList, altOk := factsRaw.([]map[string]interface{})
		if !altOk {
			return nil, fmt.Errorf("create_memory: 'facts' parameter is not a valid list (got %T)", factsRaw)
		}
		factsList = make([]interface{}, len(altList))
		for i, v := range altList {
			factsList[i] = v
		}
	}

	mgr, err := memory.New(memoryDBPath)
	if err != nil {
		return nil, fmt.Errorf("create_memory: failed to initialize memory manager: %w", err)
	}
	defer func() {
		if closeErr := mgr.Close(); closeErr != nil {
			// Log this error, but don't overwrite the original error if there was one.
			// This matches how the CLI's handleMemoryCommand does it.
			fmt.Printf("Warning: error closing memory database in createMemory tool: %v\n", closeErr)
		}
	}()

	processedIDs := []string{}

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
		// The memory package itself should handle validation of ID format if necessary,
		// but keeping basic slash check as it's a common path issue from filename-based IDs.
		if strings.Contains(idStr, "/") || strings.Contains(idStr, `\`) {
			return nil, fmt.Errorf("create_memory: fact item 'id' ('%s') cannot contain slashes due to legacy reasons, even if the memory manager might allow it. Please use simple IDs.", idStr)
		}

		if err := mgr.AddMemory(idStr, factStr); err != nil {
			return nil, fmt.Errorf("create_memory: failed to add memory for id '%s': %w", idStr, err)
		}
		processedIDs = append(processedIDs, idStr)
	}

	response := map[string]any{
		"processed_ids": processedIDs,
		// Since AddMemory is an upsert, distinguishing between "added" and "updated"
		// is not straightforward without querying before adding, which adds complexity.
		// A list of processed IDs is simpler and still informative.
	}

	return response, nil
}
