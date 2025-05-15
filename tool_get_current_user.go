package smolcode

import (
	"os/user"

	"google.golang.org/genai"
)

var GetCurrentUserTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name:        "get_current_user",
				Description: "Get the current system user.",
				Parameters:  &genai.Schema{}, // No parameters needed
			},
		},
	},
	Function: func(args map[string]any) (map[string]any, error) {
		currentUser, err := user.Current()
		if err != nil {
			return nil, err
		}
		return map[string]any{"username": currentUser.Username}, nil
	},
}
