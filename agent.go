package smolcode

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	// Used for string manipulation
	"strings"
	"syscall"
	"time"

	"google.golang.org/genai"
)

func Code(conversationFilename string) {
	initialHistory := LoadConversationFromFile(conversationFilename)
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GEMINI_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		fmt.Printf("Error initializing client: %s\n", err.Error())
	}

	scanner := bufio.NewScanner(os.Stdin)
	getUserMessage := func() (string, bool) {
		if !scanner.Scan() {
			return "", false
		}

		return scanner.Text(), true
	}

	tools := NewToolBox()
	tools.
		Add(ReadFileTool).
		Add(ListFilesTool).
		Add(EditFileTool).
		Add(CreateCheckpointTool).
		Add(ListChangesTool).
		Add(RunCommandTool)

	systemPrompt, err := readFileContent("smolcode.md")
	if err != nil {
		fmt.Printf("Error reading smolcode.md: %s\n", err.Error())
		return
	}

	agent := NewAgent(client, getUserMessage, tools, systemPrompt, initialHistory)
	if err := agent.Run(ctx); err != nil {
		fmt.Printf("Error: %s\n", err.Error())
	}
}

func NewAgent(client *genai.Client, getUserMessage func() (string, bool), tools ToolBox, systemInstruction string, initialHistory []*genai.Content) *Agent {
	return &Agent{
		client:            client,
		getUserMessage:    getUserMessage,
		tools:             tools,
		tracingEnabled:    false,
		systemInstruction: systemInstruction,
		history:           initialHistory,
	}
}

type Agent struct {
	client            *genai.Client
	getUserMessage    func() (string, bool)
	tools             ToolBox
	tracingEnabled    bool
	systemInstruction string
	history           []*genai.Content
}

func (agent *Agent) EnableTracing() *Agent {
	agent.tracingEnabled = true
	return agent
}

func (agent *Agent) DisableTracing() *Agent {
	agent.tracingEnabled = false
	return agent
}

func (agent *Agent) Run(ctx context.Context) error {
	if agent.history == nil {
		agent.history = []*genai.Content{}
	}
	fmt.Println("Chat with Gemini (use 'Ctrl-c' to quit)")
	fmt.Printf("Available tools: %s\n", strings.Join(agent.tools.Names(), ", "))
	readUserInput := true
	for {
		if readUserInput {
			fmt.Print("\u001b[94mYou\u001b[0m: ")
			userInput, ok := agent.getUserMessage()
			if !ok {
				break
			}

			if strings.TrimSpace(userInput) == "/trace" {
				agent.EnableTracing()
				continue
			}
			if strings.TrimSpace(userInput) == "/no-trace" {
				agent.DisableTracing()
				continue
			}
			if strings.TrimSpace(userInput) == "/reload" {
				err := agent.reload()
				if err != nil {
					agent.errorMessage("Failed to reload: %v", err)
				}
				// Continue the loop to allow the user to try again or enter a different command if reload fails.
				// If reload succeeds, the process is replaced by syscall.Exec, so this 'continue' is not reached.
				continue
			}
			userMessage := genai.NewContentFromText(userInput, genai.RoleUser)
			agent.history = append(agent.history, userMessage)
		}

		response, err := agent.runInference(ctx, agent.history)
		if err != nil {
			return err
		}

		if len(response.Candidates) == 0 {
			agent.errorMessage("empty response received")
			readUserInput = true
			continue
		}

		responseMessage := response.Candidates[0].Content
		if AsJSON(responseMessage.Parts) == "[{}]" {
			agent.errorMessage("empty response received")
			readUserInput = true
			continue
		}
		agent.history = append(agent.history, responseMessage)
		toolResults := []*genai.Content{}

		for _, content := range responseMessage.Parts {
			if content.Text != "" {
				fmt.Printf("\u001b[93mGemini\u001b[0m: %s\n", strings.TrimSpace(content.Text))
			} else if content.FunctionCall != nil {
				response := agent.executeTool(content.FunctionCall)
				toolResults = append(toolResults, response)
			}
		}

		if len(toolResults) == 0 {
			readUserInput = true
			continue
		}

		readUserInput = false
		agent.history = append(agent.history, toolResults...)
	}

	// Save conversation history after the loop exits
	fmt.Println("\nExiting... Saving conversation history to smolcode.json")
	historyJSON, err := json.MarshalIndent(agent.history, "", "  ")
	if err != nil {
		fmt.Printf("Error marshalling conversation history: %v\n", err)
		// Decide if we should return the error or just log it and exit cleanly
	} else {
		err = os.WriteFile("smolcode.json", historyJSON, 0644)
		if err != nil {
			fmt.Printf("Error writing conversation file smolcode.json: %v\n", err)
			// Decide if we should return the error or just log it and exit cleanly
		} else {
			fmt.Println("Conversation history saved successfully.")
		}
	}

	return nil
}

func (agent *Agent) executeTool(call *genai.FunctionCall) *genai.Content {
	agent.toolMessage("%s", FormatFunctionCall(call))
	tool, found := agent.tools.Get(call.Name)
	if !found {
		agent.toolMessage("%s", "not found")
		return genai.NewContentFromFunctionResponse(call.Name, map[string]any{"error": "tool not found"}, genai.RoleUser)
	}
	result, err := tool.Function(call.Args)
	if err != nil {
		agent.toolMessage("%s", err)
		return genai.NewContentFromFunctionResponse(call.Name, map[string]any{"error": err.Error()}, genai.RoleUser)
	}

	agent.toolMessage("%s", CropText(AsJSON(result), 70))
	return genai.NewContentFromFunctionResponse(call.Name, result, genai.RoleUser)
}

func (agent *Agent) errorMessage(fmtStr string, value ...any) {
	fmt.Printf("\u001b[91mError\u001b[0m: "+fmtStr+"\n", value...)
}

func (agent *Agent) toolMessage(fmtStr string, value ...any) {
	fmt.Printf("\u001b[95mTool\u001b[0m: "+fmtStr+"\n", value...)
}

func (agent *Agent) trace(direction string, arg any) {
	if !agent.tracingEnabled {
		return
	}
	fmt.Printf("\u001b[90mTrace %s\u001b[0m: %s\n", direction, AsJSON(arg))
}

func (agent *Agent) runInference(ctx context.Context, conversation []*genai.Content) (*genai.GenerateContentResponse, error) {
	agent.trace(">", conversation)
	response, err := agent.client.Models.GenerateContent(ctx, "gemini-2.5-pro-preview-03-25", conversation, &genai.GenerateContentConfig{
		MaxOutputTokens:   4 * 1024,
		Tools:             []*genai.Tool{agent.tools.List()},
		SystemInstruction: agent.systemPrompt(),
	})

	agent.trace("<", response)
	return response, err
}

func (agent *Agent) systemPrompt() *genai.Content {
	if strings.TrimSpace(agent.systemInstruction) == "" {
		return nil
	}

	return genai.NewContentFromText(agent.systemInstruction, genai.RoleUser)
}

func (agent *Agent) reload() error {
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("smolcode-%d.json", timestamp)

	// Serialize conversation history to JSON
	// Ensure agent.history is properly marshaled. It might need specific handling
	// if genai.Content contains complex types or interfaces not directly serializable.
	// Let's assume direct marshaling works for now, but this might need refinement.
	historyJSON, err := json.MarshalIndent(agent.history, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal conversation history: %w", err)
	}

	// Write JSON to file
	err = os.WriteFile(filename, historyJSON, 0644)
	if err != nil {
		return fmt.Errorf("failed to write conversation file %q: %w", filename, err)
	}
	fmt.Printf("Conversation saved to %s\n", filename)

	// Prepare arguments for the new process
	goCmdPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("failed to find 'go' executable: %w", err)
	}

	// Ensure the path to main.go is correct relative to the execution context
	// If running from the project root, "cmd/smolcode/main.go" should be correct.
	mainGoPath := "cmd/smolcode/main.go"

	args := []string{
		"go",
		"run",
		mainGoPath,
		"-c",
		filename,
	}

	// Use syscall.Exec to replace the current process
	fmt.Printf("Reloading with command: %s\n", strings.Join(args, " "))
	env := os.Environ()
	err = syscall.Exec(goCmdPath, args, env)
	if err != nil {
		// If syscall.Exec returns, it means an error occurred.
		return fmt.Errorf("failed to execute new process: %w", err)
	}

	// syscall.Exec should not return on success. This indicates a problem.
	return errors.New("syscall.Exec finished unexpectedly without error, which indicates a failure")
}

func readFileContent(filepath string) (string, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// File not found is not an error in this context, return empty string.
			return "", nil
		}
		// For other read errors, return the error.
		return "", fmt.Errorf("reading file %q: %w", filepath, err)
	}
	return string(content), nil
}

func AsJSON(value any) string {
	asBytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(asBytes)
}

func CropText(in string, width int) string {
	if len(in) <= width {
		return in
	}

	half := width / 2
	return in[0:half] + "…" + in[len(in)-half:]
}
