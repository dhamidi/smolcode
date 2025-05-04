package smolcode

import (
	"fmt"
	"os/exec"
	"strings"

	"google.golang.org/genai"
)

var ListChangesTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "list_changes",
				Description: strings.TrimSpace(
					`
Use this tool to receive a list of all changes in files in the project.

This input is useful for drafting a commit message for create_checkpoint.
`),
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"details": {
							Type:        genai.TypeString,
							Title:       "details",
							Description: "The level of detail to use for listing changes",
							Format:      "enum",
							Enum:        []string{"files", "diff"},
						},
					},
					Required: []string{"details"},
				},
			},
		},
	},
	Function: listChanges,
}

func listChanges(args map[string]any) (map[string]any, error) {
	details := fmt.Sprintf("%s", args["details"])
	if details == "" {
		return nil, fmt.Errorf("list_changes: no detail level specified")
	}

	gitArgs := []string{"status"}
	if details == "diff" {
		gitArgs = []string{"diff"}
	}
	if output, err := exec.Command("git", gitArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("list_changes: failed to run git %s: %w (output: %s)", strings.Join(gitArgs, " "), err, output)
	} else {
		return map[string]any{"output": string(output)}, nil
	}
}
