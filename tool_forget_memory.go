package smolcode

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dhamidi/smolcode/memory"
	"google.golang.org/genai"
)

var ForgetMemoryTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "forget_memory",
				Description: strings.TrimSpace(
					`
Forgets facts from the knowledge base using the memory manager.

Provide a list of 'factIDs' to delete multiple facts at once.
`,
				),
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"factIDs": {
							Type:        genai.TypeArray,
							Description: "List of fact IDs to forget.",
							Items: &genai.Schema{
								Type: genai.TypeString,
							},
						},
					},
					Required: []string{"factIDs"},
				},
			},
		},
	},
	Function: forgetMemory,
}

func forgetMemory(args map[string]any) (map[string]any, error) {
	factIDsRaw, ok := args["factIDs"]
	if !ok {
		return nil, fmt.Errorf("forget_memory: missing required parameter 'factIDs'")
	}

	factIDsList, ok := factIDsRaw.([]interface{}) // The genai library decodes JSON arrays as []interface{}
	if !ok {
		return nil, fmt.Errorf("forget_memory: 'factIDs' parameter is not a valid list (got %T)", factIDsRaw)
	}

	mgr, err := memory.New(memoryDBPath)
	if err != nil {
		return nil, fmt.Errorf("forget_memory: failed to initialize memory manager: %w", err)
	}
	defer func() {
		if closeErr := mgr.Close(); closeErr != nil {
			fmt.Printf("Warning: error closing memory database in forgetMemory tool: %v\n", closeErr)
		}
	}()

	processedIDs := []string{}

	for _, factIDItem := range factIDsList {
		factIDStr, ok := factIDItem.(string)
		if !ok {
			return nil, fmt.Errorf("forget_memory: fact ID item is not a valid string (got %T)", factIDItem)
		}

		if err := mgr.Forget(factIDStr); err != nil {
			if errors.Is(err, memory.ErrNotFound) {
				continue // Skip if the fact ID does not exist
			}
			return nil, fmt.Errorf("forget_memory: failed to forget memory for id '%s': %w", factIDStr, err)
		}
		processedIDs = append(processedIDs, factIDStr)
	}

	response := map[string]any{
		"processed_ids": processedIDs,
	}

	return response, nil
}
