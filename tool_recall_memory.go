package smolcode

import (
	"database/sql" // For sql.ErrNoRows
	"fmt"

	// "os"
	// "os/exec"
	// "path/filepath"
	"strings"

	"github.com/dhamidi/smolcode/memory"
	"google.golang.org/genai"
)

// const memoryDBPath = ".smolcode/memory.db" // This is already defined in tool_create_memory.go in the same package

var RecallMemoryTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "recall_memory",
				Description: strings.TrimSpace(
					`
Recalls facts from the knowledge base using the memory manager.

Either provide a specific 'factID' to retrieve a single fact, 
or provide an 'about' search term to find relevant facts using full-text search.

When searching, prefer to search with single words and narrow down as needed.
`,
				),
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"about": {
							Type:        genai.TypeString,
							Description: "A search term to find relevant facts using the memory manager's full-text search.",
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
	var about, factID string

	if aboutRaw, ok := args["about"]; ok {
		about, _ = aboutRaw.(string)
	}
	if factIDRaw, ok := args["factID"]; ok {
		factID, _ = factIDRaw.(string)
	}

	mgr, err := memory.New(memoryDBPath)
	if err != nil {
		return nil, fmt.Errorf("recall_memory: failed to initialize memory manager: %w", err)
	}
	defer func() {
		if closeErr := mgr.Close(); closeErr != nil {
			fmt.Printf("Warning: error closing memory database in recallMemory tool: %v\n", closeErr)
		}
	}()

	if factID != "" {
		// Recall by specific ID
		// Slash check is less critical here as DB will handle ID format, but kept for consistency with create_memory error message if desired.
		// However, the memory.GetMemoryByID doesn't have such restrictions internally on format of ID string itself other than what DB imposes.
		// For now, removing the slash check here as the DB lookup is the source of truth.
		mem, err := mgr.GetMemoryByID(factID)
		if err != nil {
			if err.Error() == sql.ErrNoRows.Error() || strings.Contains(err.Error(), "not found") { // memory.GetMemoryByID wraps sql.ErrNoRows
				return nil, fmt.Errorf("recall_memory: fact with ID '%s' not found", factID)
			}
			return nil, fmt.Errorf("recall_memory: error retrieving fact '%s': %w", factID, err)
		}
		return map[string]any{
			"id":   mem.ID,
			"fact": mem.Content,
		}, nil
	} else if about != "" {
		// Recall by search term using MemoryManager.SearchMemory
		if strings.TrimSpace(about) == "" {
			return nil, fmt.Errorf("recall_memory: 'about' parameter cannot be empty or only whitespace")
		}

		mems, err := mgr.SearchMemory(about)
		if err != nil {
			return nil, fmt.Errorf("recall_memory: error searching for facts about '%s': %w", about, err)
		}

		if len(mems) == 0 {
			return nil, fmt.Errorf("recall_memory: no facts found containing all words in '%s'", about)
		}

		// Convert []*memory.Memory to []map[string]string for the tool response
		matches := make([]map[string]string, len(mems))
		for i, mem := range mems {
			matches[i] = map[string]string{
				"id":   mem.ID,
				"fact": mem.Content,
			}
		}

		return map[string]any{
			"matches": matches,
		}, nil

	} else {
		return nil, fmt.Errorf("recall_memory: either 'factID' or 'about' parameter must be provided")
	}
}
