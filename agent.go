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

func Code(conversationFilename string, modelName string) {
	initialHistory := LoadConversationFromFile(conversationFilename)

	// Check if the conversation file is a temporary reload file and remove it after loading
	if conversationFilename != "" && strings.HasPrefix(conversationFilename, "smolcode-") && strings.HasSuffix(conversationFilename, ".json") {
		err := os.Remove(conversationFilename)
		if err != nil {
			// Log the error but continue execution
			fmt.Fprintf(os.Stderr, "Warning: could not remove temporary conversation file %s: %v\n", conversationFilename, err)
		} else {
			fmt.Printf("Removed temporary conversation file: %s\n", conversationFilename)
		}
	}

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
		Add(RunCommandTool).
		Add(SearchCodeTool).
		Add(CreateMemoryTool).
		Add(RecallMemoryTool).
		Add(PlannerTool)

	systemPrompt, err := readFileContent("smolcode.md")
	if err != nil {
		fmt.Printf("Error reading smolcode.md: %s\n", err.Error())
		return
	}

	agent := NewAgent(client, getUserMessage, tools, systemPrompt, initialHistory, "main")
	if modelName != "" {
		agent.ChooseModel(modelName)
	}
	if err := agent.Run(ctx); err != nil {
		fmt.Printf("Error: %s\n", err.Error())
	}
}

func NewAgent(client *genai.Client, getUserMessage func() (string, bool), tools ToolBox, systemInstruction string, initialHistory []*genai.Content, name string) *Agent {
	if name == "" {
		name = "main"
	}
	agent := &Agent{
		client:            client,
		getUserMessage:    getUserMessage,
		tools:             tools,
		tracingEnabled:    false,
		systemInstruction: systemInstruction,
		history:           initialHistory,
		name:              name,
		modelName:         "gemini-2.5-pro-preview-03-25", // Default model
		// cachedContent and systemPromptModTime are zero initially
	}

	// Caching logic has been moved to runInference

	return agent
}

type Agent struct {
	name              string
	client            *genai.Client
	getUserMessage    func() (string, bool)
	tools             ToolBox
	tracingEnabled    bool
	systemInstruction string
	history           []*genai.Content
	modelName         string
	cachedContent     string // Stores the resource name of the cached content
}

func (agent *Agent) ChooseModel(modelName string) *Agent {
	agent.modelName = modelName
	return agent
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
	fmt.Printf("Chat with %s (use 'Ctrl-c' to quit)\n", agent.modelName)
	fmt.Printf("Available tools: %s\n", strings.Join(agent.tools.Names(), ", "))
	readUserInput := true
	for {
		if readUserInput {
			fmt.Printf("\u001b[94mYou [%d]\u001b[0m: ", len(agent.history)) // Print prompt with history length
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
			// For any other error, return it to terminate the agent run
			return err
		}

		// Print usage metadata summary
		fmt.Printf("\u001b[90m%s\u001b[0m\n", formatUsageMetadata(response.UsageMetadata))

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
				agent.geminiMessage("%s", content.Text)
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
	fmt.Printf("\u001b[91mError [%d]\u001b[0m: "+fmtStr+"\n", append([]any{len(agent.history)}, value...)...)
}

func (agent *Agent) youMessage(fmtStr string, value ...any) {
	// This function is now the primary way to show user input, but the original direct fmt.Print in Run should handle the initial prompt.
	// We format it consistently here. Note: the user input itself is added to history *before* calling inference,
	// so the len(agent.history) here might be off by one depending on exact call order. Let's assume it reflects the state *before* this message.
	// Re-checking Run loop logic: history is appended *after* reading input. So len(agent.history) should be correct *when* this is called.
	fmt.Printf("\u001b[94mYou [%d]\u001b[0m: %s\n", len(agent.history)-1, fmt.Sprintf(fmtStr, value...)) // Use len-1 assuming history includes current msg
}

func (agent *Agent) toolMessage(fmtStr string, value ...any) {
	fmt.Printf("\u001b[95mTool [%d]\u001b[0m: "+fmtStr+"\n", append([]any{len(agent.history)}, value...)...)
}

func (agent *Agent) geminiMessage(fmtStr string, value ...any) {
	fmt.Printf("\u001b[93mGemini [%d]\u001b[0m: "+fmtStr+"\n", append([]any{len(agent.history)}, value...)...)
}

func (agent *Agent) trace(direction string, arg any) {
	if !agent.tracingEnabled {
		return
	}
	// Trace doesn't strictly add to the conversation history, so maybe we don't include length?
	// For consistency, let's add it.
	fmt.Printf("\u001b[90mTrace [%d] %s\u001b[0m: %s\n", len(agent.history), direction, AsJSON(arg))
}

func (agent *Agent) runInference(ctx context.Context, conversation []*genai.Content) (*genai.GenerateContentResponse, error) {
	agent.trace(">", conversation)

	// --- Caching Logic Start ---
	// Create a new cache for the current conversation history for this turn.
	var turnCacheName string
	// SystemInstruction and Tools are always included in the turn cache if available.
	cacheConfig := &genai.CreateCachedContentConfig{
		DisplayName: fmt.Sprintf("smolcode-turncache-%s-%d", agent.name, time.Now().UnixNano()),
		TTL:         5 * time.Minute,
		// Model:    agent.modelName, // This was incorrect, Model is part of Caches.Create call
	}

	if agent.systemInstruction != "" {
		cacheConfig.SystemInstruction = agent.systemPrompt()
	}
	if len(agent.tools) > 0 {
		cacheConfig.Tools = []*genai.Tool{agent.tools.List()}
	}
	// The conversation (history) is added to the *contents* of the cache.
	if len(conversation) > 0 {
		cacheConfig.Contents = conversation // Cache the current conversation (history)
	}

	cachedContentInstance, createErr := agent.client.Caches.Create(ctx, agent.modelName, cacheConfig)
	if createErr != nil {
		// fmt.Fprintf(os.Stderr, "Warning: could not create turn-specific cached content for agent %s: %v\n", agent.name, createErr)
		agent.trace("CacheCreate", map[string]string{"status": "error", "agent": agent.name, "error": createErr.Error()})
		// Proceed without this turn's cache if creation fails
	} else {
		turnCacheName = cachedContentInstance.Name
		// fmt.Printf("INFO: Created turn-specific cache: %s for agent %s\n", turnCacheName, agent.name)
		agent.trace("CacheCreate", map[string]string{"status": "success", "cacheName": turnCacheName, "agent": agent.name})
		// Defer deletion of this turn-specific cache
		defer func() {
			if turnCacheName != "" {
				// fmt.Printf("INFO: Deleting turn-specific cache: %s for agent %s\n", turnCacheName, agent.name)
				agent.trace("CacheDeleteAttempt", map[string]string{"cacheName": turnCacheName, "agent": agent.name})
				_, delErr := agent.client.Caches.Delete(context.Background(), turnCacheName, nil)
				if delErr != nil {
					// fmt.Fprintf(os.Stderr, "Warning: could not delete turn-specific cached content %s for agent %s: %v\n", turnCacheName, agent.name, delErr)
					agent.trace("CacheDelete", map[string]string{"status": "error", "cacheName": turnCacheName, "agent": agent.name, "error": delErr.Error()})
				} else {
					// fmt.Printf("INFO: Successfully deleted turn-specific cache: %s for agent %s\n", turnCacheName, agent.name)
					agent.trace("CacheDelete", map[string]string{"status": "success", "cacheName": turnCacheName, "agent": agent.name})
				}
			}
		}()
	}
	// --- Caching Logic End ---

	var response *genai.GenerateContentResponse
	var err error

	retryDelays := []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second, 30 * time.Second}
	maxRetries := 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		config := &genai.GenerateContentConfig{
			MaxOutputTokens: 4 * 1024,
		}

		if turnCacheName != "" {
			config.CachedContent = turnCacheName
			// When using a valid turn-specific cache (turnCacheName is not empty),
			// SystemInstruction, Tools, and Contents (history) are already part of the cache.
			// So, we do NOT set them again in the config here for the GenerateContent call.
			// The conversation argument to GenerateContent will be ignored by the backend if CachedContent is set.
		} else {
			// Only set these if not using a turn-specific cache (e.g., cache creation failed)
			if len(agent.tools) > 0 {
				config.Tools = []*genai.Tool{agent.tools.List()}
			}
			config.SystemInstruction = agent.systemPrompt()
			// Note: The 'conversation' argument to GenerateContent will be used directly in this case.
		}

		agent.trace("GenerateContentConfig", config) // Log the config being used
		response, err = agent.client.Models.GenerateContent(ctx, agent.modelName, conversation, config)

		if err == nil {
			agent.trace("<", response)
			return response, nil // Success
		}

		// Check if the error is a 500 error or similar that might benefit from a retry
		// This is a basic check; you might want to make it more specific
		if strings.Contains(err.Error(), "An internal error has occurred") || strings.Contains(err.Error(), "server error") {
			fmt.Fprintf(os.Stderr, "Attempt %d/%d: Encountered API error: %v\n", attempt+1, maxRetries, err)
			if attempt < len(retryDelays) {
				delay := retryDelays[attempt]
				fmt.Fprintf(os.Stderr, "Retrying in %s...\n", delay)
				time.Sleep(delay)
			} else if attempt < maxRetries-1 {
				// If we've exhausted specific delays but not max retries, use the last delay value
				delay := retryDelays[len(retryDelays)-1]
				fmt.Fprintf(os.Stderr, "Retrying in %s...\n", delay)
				time.Sleep(delay)
			} else {
				// Last attempt failed
				fmt.Fprintf(os.Stderr, "All %d retry attempts failed.\n", maxRetries)
				break
			}
		} else {
			// Non-retryable error
			agent.trace("<", response) // Trace the error response if any
			return response, err
		}
	}

	// If all retries fail, return the last error
	agent.trace("<", response) // Trace the final error response if any
	return response, fmt.Errorf("after %d attempts, last error: %w", maxRetries, err)
}

func (agent *Agent) systemPrompt() *genai.Content {
	if strings.TrimSpace(agent.systemInstruction) == "" {
		return nil
	}

	return genai.NewContentFromText(agent.systemInstruction, genai.RoleUser)
}

// buildProject executes a hardcoded build command.
func (agent *Agent) buildProject() error {
	agent.geminiMessage("Attempting to build the project...")

	// Hardcoded build command
	fullBuildCommand := "go build -tags fts5 -o smolcode cmd/smolcode/main.go"

	agent.geminiMessage("Executing build command: %s", fullBuildCommand)

	parts := strings.Fields(fullBuildCommand)
	if len(parts) == 0 {
		return fmt.Errorf("buildProject: hardcoded build command is effectively empty after splitting")
	}
	cmdName := parts[0]
	cmdArgs := parts[1:]

	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Stdout = os.Stdout // Pipe build output to agent's stdout
	cmd.Stderr = os.Stderr // Pipe build errors to agent's stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("buildProject: build failed: %w", err)
	}

	agent.geminiMessage("Project built successfully.")
	return nil
}

func (agent *Agent) reload() error {
	// First, try to build the project
	if err := agent.buildProject(); err != nil {
		// If build fails, return the error and don't proceed with reload
		return fmt.Errorf("project build failed, aborting reload: %w", err)
	}

	// If build is successful, proceed with saving state and reloading
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

// formatUsageMetadata creates a single-line summary of token usage.
// It highlights the prompt token count relative to the maximum allowed (1,048,576).
// It also indicates if cached content was used for the request.
func formatUsageMetadata(metadata *genai.GenerateContentResponseUsageMetadata) string {
	if metadata == nil {
		return "Usage metadata not available."
	}
	// The input token limit is 1,048,576
	limit := 1048576
	cacheInfo := ""
	if metadata.CachedContentTokenCount > 0 {
		// If CachedContentTokenCount is greater than 0, it implies a cache was hit.
		// The API does not directly return the cache name in UsageMetadata,
		// but we can infer its use from this field.
		cacheInfo = fmt.Sprintf(", CachedContentTokens=%d (Cache Hit)", metadata.CachedContentTokenCount)
	} else if metadata.PromptTokenCount > 0 {
		// If PromptTokenCount > 0 and CachedContentTokenCount is 0, it implies cache was not used for prompt tokens.
		cacheInfo = " (Cache Miss/Not Used)"
	}

	return fmt.Sprintf(
		"Token Usage: Prompt=%d/%d (%d%%)%s, Candidates=%d, Total=%d",
		metadata.PromptTokenCount,
		limit,
		(metadata.PromptTokenCount*100)/int32(limit), // Calculate percentage
		cacheInfo,
		metadata.CandidatesTokenCount,
		metadata.TotalTokenCount,
	)
}

func CropText(in string, width int) string {
	if len(in) <= width {
		return in
	}

	half := width / 2
	return in[0:half] + "…" + in[len(in)-half:]
}
