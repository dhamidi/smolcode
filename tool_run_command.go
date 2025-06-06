package smolcode

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"google.golang.org/genai"
)

var RunCommandTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "run_command",
				Description: strings.TrimSpace(`
Run a terminal command. Only use this for short-running commands.
Do not use this for interactive commands.`),
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"command": {
							Type:        genai.TypeString,
							Description: "The command to run.",
						},
					},
					Required: []string{"command"},
				},
			},
		},
	},
	Function: runCommand,
}

func runCommand(args map[string]any) (map[string]any, error) {
	command := fmt.Sprintf("%s", args["command"])
	if command == "" {
		return nil, fmt.Errorf("run_command: no command specified")
	}

	cmdParts := strings.Fields(command)
	if len(cmdParts) == 0 {
		return nil, fmt.Errorf("run_command: no command specified")
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh" // Default to sh if SHELL is not set
	}
	cmd := exec.Command(shell, "-c", command)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("run_command: failed to run command '%s': %w (output: %s)", command, err, output)
	} else {
		return map[string]any{"output": string(output)}, nil
	}
}
