package smolcode // Changed from main

import (
	"time"

	"google.golang.org/genai"
)

var DateTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "get_current_date",
				Description: "Returns the current date formatted as YYYY-MM-DD.",
				// No parameters needed for this tool
				Parameters: &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{}},
			},
		},
	},
	Function: func(args map[string]any) (map[string]any, error) {
		// This tool takes no arguments, so 'args' is ignored.
		currentDate := time.Now().Format("2006-01-02") // YYYY-MM-DD format
		return map[string]any{"current_date": currentDate}, nil
	},
}
