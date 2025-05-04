package smolcode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/genai"
)

func Code() {
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
	tools.Add(ReadFileTool).Add(ListFilesTool).Add(EditFileTool)
	agent := NewAgent(client, getUserMessage, tools)
	if err := agent.Run(ctx); err != nil {
		fmt.Printf("Error: %s\n", err.Error())
	}
}

func NewAgent(client *genai.Client, getUserMessage func() (string, bool), tools ToolBox) *Agent {
	return &Agent{
		client:         client,
		getUserMessage: getUserMessage,
		tools:          tools,
		tracingEnabled: false,
	}
}

type Agent struct {
	client         *genai.Client
	getUserMessage func() (string, bool)
	tools          ToolBox
	tracingEnabled bool
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
	conversation := []*genai.Content{}
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
			userMessage := genai.NewContentFromText(userInput, genai.RoleUser)
			conversation = append(conversation, userMessage)
		}

		response, err := agent.runInference(ctx, conversation)
		if err != nil {
			return err
		}

		if len(response.Candidates) == 0 {
			fmt.Printf("\u001b[91mError\u001b[0m: empty response received\n")
			readUserInput = true
			continue
		}

		responseMessage := response.Candidates[0].Content
		conversation = append(conversation, responseMessage)
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
		conversation = append(conversation, toolResults...)
	}

	return nil
}

func (agent *Agent) executeTool(call *genai.FunctionCall) *genai.Content {
	fmt.Printf("\u001b[95mTool\u001b[0m: %s\n", FormatFunctionCall(call))
	tool, found := agent.tools.Get(call.Name)
	if !found {
		fmt.Printf("\u001b[95mTool\u001b[0m: %s\n", "not found")
		return genai.NewContentFromFunctionResponse(call.Name, map[string]any{"error": "tool not found"}, genai.RoleUser)
	}
	result, err := tool.Function(call.Args)
	if err != nil {
		fmt.Printf("\u001b[95mTool\u001b[0m: %s\n", err)
		return genai.NewContentFromFunctionResponse(call.Name, map[string]any{"error": err.Error()}, genai.RoleUser)
	}

	fmt.Printf("\u001b[95mTool\u001b[0m: %s\n", CropText(AsJSON(result), 70))
	return genai.NewContentFromFunctionResponse(call.Name, result, genai.RoleUser)
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
		MaxOutputTokens: 1024,
		Tools:           []*genai.Tool{agent.tools.List()},
	})

	agent.trace("<", response)
	return response, err
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
