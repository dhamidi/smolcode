package smolcode

import (
	"fmt"
	"os/exec"
	"strings"

	"google.golang.org/genai"
)

var CreateCheckpointTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "create_checkpoint",
				Description: strings.TrimSpace(
					`
Store a summary of recent changes in git with the given commit message.

Commit messages MUST follow the conventional commit format:

<type>[optional scope]: <description>

[optional body]

[optional footer(s)]

The commit contains the following structural elements, to communicate intent to the consumers of your library:

fix: a commit of the type fix patches a bug in your codebase (this correlates with PATCH in Semantic Versioning).
feat: a commit of the type feat introduces a new feature to the codebase (this correlates with MINOR in Semantic Versioning).
BREAKING CHANGE: a commit that has a footer BREAKING CHANGE:, or appends a ! after the type/scope, introduces a breaking API change (correlating with MAJOR in Semantic Versioning). A BREAKING CHANGE can be part of commits of any type.
types other than fix: and feat: are allowed, for example @commitlint/config-conventional (based on the Angular convention) recommends build:, chore:, ci:, docs:, style:, refactor:, perf:, test:, and others.
footers other than BREAKING CHANGE: <description> may be provided and follow a convention similar to git trailer format.
`),
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"message": {
							Type:        genai.TypeString,
							Title:       "message",
							Description: "The conventional commit message to use for this commit",
						},
					},
					Required: []string{"message"},
				},
			},
		},
	},
	Function: commitChanges,
}

func commitChanges(args map[string]any) (map[string]any, error) {
	message := fmt.Sprintf("%s", args["message"])
	if message == "" {
		return nil, fmt.Errorf("create_checkpoint: no commit message provided")
	}

	if err := exec.Command("git", "add", ".").Run(); err != nil {
		return nil, fmt.Errorf("create_checkpoint: failed to stage files: %w", err)
	}
	if output, err := exec.Command("git", "commit", "-m", message).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("create_checkpoint: failed to commit: %w (output: %s)", err, output)
	} else {
		return map[string]any{"output": string(output)}, nil
	}
}
