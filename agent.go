package smolcode

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	// Used for string manipulation
	"strings"
	"syscall"
	"time"

	"github.com/dhamidi/smolcode/history"
	"google.golang.org/genai"
)

//go:embed .smolcode/system.md
var defaultSystemPrompt string

func Code(conversationID string, modelName string, newConversationFlag bool) error {
	var loadedConv *history.Conversation
	var err error
	initialHistoryForAgent := []*genai.Content{}
	var conversationWasNewlyCreated bool // Added to track if conversation is new

	if conversationID != "" {
		// Attempt to load the specified conversation
		fmt.Printf("Attempting to load conversation with ID: %s\n", conversationID)
		loadedConv, err = history.Load(conversationID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading conversation %s: %v. Starting a new conversation instead.\n", conversationID, err)
			// Fall through to creating a new conversation
			loadedConv, err = history.New()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Fatal: Could not create new conversation: %v\n", err)
				return err // Return the error
			}
			conversationWasNewlyCreated = true
		} else {
			conversationWasNewlyCreated = false
		}
	} else if newConversationFlag {
		// Explicitly start a new conversation
		loadedConv, err = history.New()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Fatal: Could not create new conversation: %v\n", err)
			return err // Return the error
		}
		conversationWasNewlyCreated = true
	} else {
		// Attempt to load the latest conversation
		fmt.Println("No conversation ID specified, attempting to load the latest conversation...")
		convList, listErr := history.ListConversations(history.DefaultDatabasePath)
		if listErr != nil {
			fmt.Fprintf(os.Stderr, "Error listing conversations: %v. Starting a new conversation.\n", listErr)
		}
		if len(convList) > 0 {
			latestID := convList[0].ID // Assumes list is sorted by latest
			// fmt.Printf("Found latest conversation with ID: %s. Attempting to load.\n", latestID) // Removed
			loadedConv, err = history.Load(latestID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading latest conversation %s: %v. Starting a new conversation instead.\n", latestID, err)
				// conversationWasNewlyCreated will be handled if a new one is created below
			} else {
				conversationWasNewlyCreated = false
				// fmt.Printf("Successfully loaded latest conversation ID: %s\n", loadedConv.ID) // Removed
			}
		} else {
			fmt.Println("No existing conversations found.")
		}
		// If loadedConv is still nil (no latest found or error loading it), create a new one
		if loadedConv == nil {
			fmt.Println("Starting a new conversation.")
			loadedConv, err = history.New()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Fatal: Could not create new conversation: %v\n", err)
				return err // Return the error
			}
			conversationWasNewlyCreated = true
		}
	}

	// Populate initialHistoryForAgent from loadedConv.Messages
	if loadedConv != nil && loadedConv.Messages != nil {
		for _, msgWrapper := range loadedConv.Messages {
			if payloadBytes, ok := msgWrapper.Payload.([]byte); ok {
				var contentPart genai.Content
				unmarshalErr := json.Unmarshal(payloadBytes, &contentPart)
				if unmarshalErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not unmarshal genai.Content from DB bytes: %v\n", unmarshalErr)
					continue
				}
				initialHistoryForAgent = append(initialHistoryForAgent, &contentPart)
			} else {
				// This case implies that the payload stored in the DB (and loaded by history.Load)
				// was not []byte. This could happen if old data exists or if there's a mismatch
				// in saving logic. For robustness, try the map[string]interface{} conversion as a fallback.
				// fmt.Fprintf(os.Stderr, "Warning: message payload in DB was not []byte (type: %T). Attempting fallback conversion.\n", msgWrapper.Payload) // Commented out
				var contentPart genai.Content
				fallbackPayloadBytes, marshalErr := json.Marshal(msgWrapper.Payload) // marshal the map/value
				if marshalErr != nil {
					// fmt.Fprintf(os.Stderr, "Warning: fallback - could not marshal message payload for history: %v\n", marshalErr) // Commented out
					continue
				}
				unmarshalErr := json.Unmarshal(fallbackPayloadBytes, &contentPart) // unmarshal into typed struct
				if unmarshalErr != nil {
					// fmt.Fprintf(os.Stderr, "Warning: fallback - could not unmarshal message payload into genai.Content: %v\n", unmarshalErr) // Commented out
					continue
				}
				initialHistoryForAgent = append(initialHistoryForAgent, &contentPart)
			}

		}
		// fmt.Printf("Loaded %d messages into agent history.\n", len(initialHistoryForAgent)) // Removed
	}
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GEMINI_API_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		fmt.Printf("Error initializing genai client: %s\n", err.Error())
		return err // Propagate error
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
		Add(WriteFileTool).
		Add(CreateCheckpointTool).
		Add(ListChangesTool).
		Add(RunCommandTool).
		Add(SearchCodeTool).
		Add(CreateMemoryTool).
		Add(RecallMemoryTool).
		Add(ForgetMemoryTool).
		Add(PlannerTool).
		Add(CodegenTool)
	systemPrompt, err := readFileContent(".smolcode/system.md")
	if err != nil {
		fmt.Printf("Error reading system.md: %s\n", err.Error())
		return err // Propagate error
	}

	agent := NewAgent(client, getUserMessage, tools, systemPrompt, initialHistoryForAgent, loadedConv, "main", loadedConv.ID, len(initialHistoryForAgent), conversationWasNewlyCreated)
	if modelName != "" {
		agent.ChooseModel(modelName)
	}
	if err := agent.Run(ctx); err != nil {
		fmt.Printf("Error running agent: %s\n", err.Error())
		// Potentially return this error if Code() should propagate agent.Run errors
	}
	return nil // Successful completion of Code function
}

func NewAgent(client *genai.Client, getUserMessage func() (string, bool), tools ToolBox, systemInstruction string, initialHistory []*genai.Content, convData *history.Conversation, name string, initialConvID string, initialLoadedMessages int, initialConvIsNew bool) *Agent {
	if name == "" {
		name = "main"
	}
	agent := &Agent{
		client:                client,
		getUserMessage:        getUserMessage,
		tools:                 tools,
		tracingEnabled:        false,
		systemInstruction:     systemInstruction,
		history:               initialHistory,
		name:                  name,
		modelName:             "gemini-2.5-pro-preview-03-25", // Default model
		displayer:             &GlamourousTextDisplay{},       // Use GlamourousTextDisplay by default
		initialConvID:         initialConvID,                  // Store passed-in value
		initialLoadedMessages: initialLoadedMessages,          // Store passed-in value
		initialConvIsNew:      initialConvIsNew,               // Store passed-in value
		// cachedContent and systemPromptModTime are zero initially
	}

	// convData is expected to be non-nil, as Code() is responsible for loading or creating it.
	// If Code() failed to provide a valid history.Conversation, it should have returned an error.
	if convData == nil {
		// This should not happen if Code() is functioning correctly.
		// Log a critical error and a (nil) agent will be returned, likely leading to a crash.
		// This indicates a programming error in the calling sequence.
		fmt.Fprintf(os.Stderr, "CRITICAL: NewAgent received nil convData. This should be handled by the caller.\n")
		// Allow agent to be returned, but persistentConversation will be nil, causing issues later.
		// Ideally, NewAgent could return an error too, or Code() ensures this is impossible.
	}
	agent.persistentConversation = convData

	// Caching logic has been moved to runInference (see runInference func)

	return agent
}

type Agent struct {
	initialConvID          string // Added to store initial conversation ID
	initialLoadedMessages  int    // Added to store count of loaded messages
	initialConvIsNew       bool   // Added to store if the conversation was new
	name                   string
	client                 *genai.Client
	getUserMessage         func() (string, bool)
	tools                  ToolBox
	tracingEnabled         bool
	systemInstruction      string
	history                []*genai.Content
	modelName              string
	cachedContent          string                // Stores the resource name of the cached content
	cachedHistoryCount     int                   // Number of history entries in cachedContent
	persistentConversation *history.Conversation // For storing history in SQLite
	displayer              TextDisplayer         // For displaying text to the user
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

func (agent *Agent) refreshCache(ctx context.Context) {
	// Only refresh cache if there's history and (no cache exists or history has grown)
	if len(agent.history) == 0 || (agent.cachedContent != "" && len(agent.history) == agent.cachedHistoryCount) {
		// No history to cache, or cache is up-to-date
		return
	}

	// Delete old cache if it exists
	if agent.cachedContent != "" {
		agent.trace("CacheDeleteAttempt", map[string]string{"cacheName": agent.cachedContent, "agent": agent.name, "reason": "refreshing"})
		_, delErr := agent.client.Caches.Delete(context.Background(), agent.cachedContent, nil) // Use a background context for deletion
		if delErr != nil {
			agent.trace("CacheDelete", map[string]string{"status": "error", "cacheName": agent.cachedContent, "agent": agent.name, "error": delErr.Error()})
			// Log error but proceed, as we'll try to create a new one.
			// fmt.Fprintf(os.Stderr, "Warning: could not delete old cached content %s for agent %s: %v\n", agent.cachedContent, agent.name, delErr)
		} else {
			agent.trace("CacheDelete", map[string]string{"status": "success", "cacheName": agent.cachedContent, "agent": agent.name})
		}
		agent.cachedContent = "" // Clear old cache name
		agent.cachedHistoryCount = 0
	}

	cacheConfig := &genai.CreateCachedContentConfig{
		DisplayName: fmt.Sprintf("smolcode-cache-%s-%d", agent.name, time.Now().UnixNano()),
		TTL:         15 * time.Minute, // Longer TTL for a persistent cache
		// Model is part of Caches.Create call
	}

	if agent.systemInstruction != "" {
		cacheConfig.SystemInstruction = agent.systemPrompt()
	}
	if len(agent.tools) > 0 {
		cacheConfig.Tools = []*genai.Tool{agent.tools.List()}
	}

	// Cache the current full history
	if len(agent.history) > 0 {
		cacheConfig.Contents = agent.history
	} else {
		// Cannot create an empty cache, or a cache with only system instruction/tools.
		// The API requires at least one Content object if SystemInstruction and Tools are not set.
		// If history is empty, and we only have sys instruction/tools, it might be better to not cache.
		// For now, we'll let it try and handle potential errors.
		// Or, more robustly, only proceed if there's something concrete to cache (history, or sys instruct + tools)
		if cacheConfig.SystemInstruction == nil && len(cacheConfig.Tools) == 0 {
			agent.trace("CacheCreateSkip", map[string]string{"reason": "nothing to cache", "agent": agent.name})
			return
		}
	}

	cachedContentInstance, createErr := agent.client.Caches.Create(ctx, agent.modelName, cacheConfig)
	if createErr != nil {
		agent.trace("CacheCreate", map[string]string{"status": "error", "agent": agent.name, "error": createErr.Error()})
		// fmt.Fprintf(os.Stderr, "Warning: could not create cached content for agent %s: %v\n", agent.name, createErr)
		agent.cachedContent = ""
		agent.cachedHistoryCount = 0
	} else {
		agent.cachedContent = cachedContentInstance.Name
		agent.cachedHistoryCount = len(agent.history) // Record how much history is in this new cache
		agent.trace("CacheCreate", map[string]string{"status": "success", "cacheName": agent.cachedContent, "agent": agent.name, "historyCount": fmt.Sprintf("%d", agent.cachedHistoryCount)})
	}
}

func (agent *Agent) Run(ctx context.Context) error {
	if agent.history == nil {
		agent.history = []*genai.Content{}
	}

	// Display initial conversation status
	if agent.initialConvIsNew {
		agent.displayer.Display(fmt.Sprintf("Started new conversation with ID: %s", agent.initialConvID))
	} else {
		agent.displayer.Display(fmt.Sprintf("Successfully loaded conversation ID: %s", agent.initialConvID))
	}
	agent.displayer.Display(fmt.Sprintf("Loaded %d messages into agent history.", agent.initialLoadedMessages))
	// End display initial conversation status

	agent.displayer.Display(fmt.Sprintf("Chat with %s (use 'Ctrl-c' to quit)", agent.modelName))
	agent.displayer.Display(fmt.Sprintf("Available tools: %s", strings.Join(agent.tools.Names(), ", ")))
	readUserInput := true
	for {
		if readUserInput {
			agent.refreshCache(ctx) // Refresh cache before getting user input

			agent.displayer.DisplayPrompt("\u001b[94mYou [%d]\u001b[0m: ", len(agent.history)) // Print prompt with history length
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
			if isContentEmpty(userMessage) {
				agent.skipMessage("User input is empty, not adding to history.")
				// Continue to next iteration to re-prompt user, skip inference for empty input
				readUserInput = true
				continue
			} else {
				agent.history = append(agent.history, userMessage)
				if err := agent.persistFullConversationToDB(); err != nil {
					// Log error, but continue. The primary history is in memory.
					fmt.Fprintf(os.Stderr, "Warning: failed to persist conversation after user message: %v\n", err)
				}
			}
		}

		response, err := agent.runInference(ctx, agent.history)
		if err != nil {
			// For any other error, return it to terminate the agent run
			return err
		}

		// Print usage metadata summary
		agent.displayer.Display(fmt.Sprintf("\u001b[90m%s\u001b[0m", formatUsageMetadata(response.UsageMetadata)))

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
		if isContentEmpty(responseMessage) {
			agent.skipMessage("Model response is empty, not adding to history.")
		} else {
			agent.history = append(agent.history, responseMessage)
			if err := agent.persistFullConversationToDB(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to persist conversation after model response: %v\n", err)
			}
		}
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
		// Filter out empty tool results before appending
		var validToolResults []*genai.Content
		skippedToolResults := 0
		for _, tr := range toolResults {
			if isContentEmpty(tr) {
				skippedToolResults++
			} else {
				validToolResults = append(validToolResults, tr)
			}
		}
		if skippedToolResults > 0 {
			agent.skipMessage("%d tool result(s) were empty and not added to history.", skippedToolResults)
		}
		if len(validToolResults) > 0 {
			agent.history = append(agent.history, validToolResults...)
			if err := agent.persistFullConversationToDB(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to persist conversation after tool results: %v\n", err)
			}
		}
	}

	// Final save of conversation to database on exit
	agent.displayer.Display("\nExiting... ensuring conversation is saved to database.")
	if err := agent.persistFullConversationToDB(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: final attempt to persist conversation to DB failed: %v\n", err)
	} else {
		agent.displayer.Display("Conversation saved to database successfully.")
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
	agent.displayer.DisplayError(fmtStr, value...)
}

func (agent *Agent) youMessage(fmtStr string, value ...any) {
	// The history length for "You" messages is usually len(agent.history)-1 when it's about the *current* user input.
	// However, the user prompt like "You [0]:" is handled separately in the Run loop.
	// This specific youMessage function doesn't seem to be called in the current codebase provided.
	// If it were to be used for general "You" messages, it would be:
	// agent.displayer.DisplayMessage("You", "94m", len(agent.history)-1, fmtStr, value...)
	// For now, let's make it use the displayer but acknowledge its current non-use or specific context.
	// It's safer to keep the history logic as it was, if it was specific.
	// The primary prompt for user input is in the `Run` loop directly using fmt.Printf.
	// Given this, DisplayMessage might not be the perfect fit if `len(agent.history)-1` is crucial and specific.
	// However, to adhere to the refactoring goal, we'll use the displayer.
	// The original fmt.Printf for the prompt in `Run` will be addressed separately.
	agent.displayer.DisplayMessage("You", "94m", len(agent.history)-1, fmtStr, value...)
}

func (agent *Agent) toolMessage(fmtStr string, value ...any) {
	agent.displayer.DisplayMessage("Tool", "95m", len(agent.history), fmtStr, value...)
}

func (agent *Agent) geminiMessage(fmtStr string, value ...any) {
	agent.displayer.DisplayMessage("Gemini", "93m", len(agent.history), fmtStr, value...)
}

func (agent *Agent) skipMessage(fmtStr string, value ...any) {
	agent.displayer.DisplayMessage("Skip  ", "96m", len(agent.history), fmtStr, value...)
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

	var response *genai.GenerateContentResponse
	var err error

	retryDelays := []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second, 30 * time.Second}
	maxRetries := 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		config := &genai.GenerateContentConfig{
			MaxOutputTokens: 8 * 1024,
		}

		var conversationToSend []*genai.Content
		// Determine if we can use the persistent cache
		usePersistentCache := agent.cachedContent != "" && agent.cachedHistoryCount > 0 && len(conversation) > agent.cachedHistoryCount

		if usePersistentCache {
			config.CachedContent = agent.cachedContent
			// SystemInstruction and Tools are already part of agent.cachedContent, so they are not set here.
			conversationToSend = conversation[agent.cachedHistoryCount:]
			agent.trace("CacheInfo", map[string]string{"status": "using_persistent_cache", "cacheName": agent.cachedContent, "cachedHistoryCount": fmt.Sprintf("%d", agent.cachedHistoryCount), "currentHistoryCount": fmt.Sprintf("%d", len(conversation))})
		} else {
			// No valid persistent cache to use, or history hasn't grown enough.
			// Send SystemInstruction and Tools explicitly.
			if len(agent.tools) > 0 {
				config.Tools = []*genai.Tool{agent.tools.List()}
			}
			config.SystemInstruction = agent.systemPrompt()
			conversationToSend = conversation // Send the full conversation
			agent.trace("CacheInfo", map[string]string{"status": "not_using_persistent_cache", "reason": "no valid cache or history not grown", "cachedContent": agent.cachedContent, "cachedHistoryCount": fmt.Sprintf("%d", agent.cachedHistoryCount), "currentHistoryCount": fmt.Sprintf("%d", len(conversation))})
		}
		agent.trace("GenerateContentConfig", config) // Log the config being used
		// Pass conversationToSend instead of the original 'conversation'
		response, err = agent.client.Models.GenerateContent(ctx, agent.modelName, conversationToSend, config)

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
			// Check for specific 403 CachedContent error using structured error fields
			var apiErr *genai.APIError
			if errors.As(err, &apiErr) {
				// instruction.md suggests genai.APIError has a Code field (int) and Message field (string).
				if apiErr.Code == 403 && strings.Contains(apiErr.Message, "CachedContent") {
					agent.trace("CachedContentError", map[string]string{"status": "ignoring_403_cached_content", "code": fmt.Sprintf("%d", apiErr.Code), "message": apiErr.Message})
					fmt.Fprintf(os.Stderr, "Encountered API Error Code %d with CachedContent: %s. Invalidating cache and retrying without cache for this attempt.\n", apiErr.Code, apiErr.Message)
					agent.cachedContent = "" // Invalidate cache
					agent.cachedHistoryCount = 0
					// Continue the loop to retry without cache for this specific attempt.
					// The next iteration of the loop in runInference will not use cache.
					continue // This will go to the next attempt in the retry loop.
				}
			}

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

	// If build is successful, proceed with saving state to DB and reloading
	agent.geminiMessage("Build successful. Ensuring current conversation is saved to database before reload...")
	if err := agent.persistFullConversationToDB(); err != nil {
		return fmt.Errorf("failed to persist conversation to DB before reload: %w", err)
	}
	agent.geminiMessage("Conversation state saved. Current conversation ID: %s", agent.persistentConversation.ID)

	// Prepare arguments for the new process
	goCmdPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("failed to find 'go' executable: %w", err)
	}

	// Ensure the path to main.go is correct relative to the execution context
	mainGoPath := "cmd/smolcode/main.go"

	args := []string{
		"go",
		"run",
		"-tags",
		"fts5",
		mainGoPath,
		"-conversation-id",              // New flag
		agent.persistentConversation.ID, // Pass the current conversation ID
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
			// File not found, return default prompt if filepath is the target system prompt file.
			if filepath == ".smolcode/system.md" {
				return defaultSystemPrompt, nil
			}
			return "", nil // Return empty for other non-existent files
		}
		// For other read errors, return the error.
		return "", fmt.Errorf("reading file %q: %w", filepath, err)
	}
	if len(strings.TrimSpace(string(content))) == 0 && filepath == ".smolcode/system.md" {
		return defaultSystemPrompt, nil // Return default if file is empty
	}
	return string(content), nil
}

// isContentEmpty checks if a genai.Content is effectively empty.
func isContentEmpty(content *genai.Content) bool {
	if content == nil || len(content.Parts) == 0 {
		return true
	}
	for _, part := range content.Parts {
		// We need to cast part to genai.Part if it's an interface.
		// Assuming part is already of a concrete type that has the fields.
		// If genai.Part is an interface, this might need adjustment.
		// For now, let's assume it's a struct or a pointer to one, accessible directly.
		if !isPartEmpty(part) { // No cast needed if content.Parts is already []genai.Part
			return false
		}
	}
	return true
}

// isPartEmpty checks if a genai.Part is effectively empty.
func isPartEmpty(part *genai.Part) bool {
	// Check all fields of genai.Part that can hold content.
	// Note: This needs to be updated if genai.Part adds new content fields.
	return part.Text == "" &&
		part.InlineData == nil &&
		part.FileData == nil &&
		part.FunctionCall == nil &&
		part.FunctionResponse == nil
}

// persistFullConversationToDB saves the current in-memory agent.history to the SQLite database.
func (agent *Agent) persistFullConversationToDB() error {
	if agent.persistentConversation == nil {
		// This might happen if history.New() failed in the constructor.
		// Or if we decide not to use persistent history for some agents.
		agent.trace("PersistToDB", map[string]string{"status": "skipped", "reason": "persistentConversation is nil"})
		return fmt.Errorf("cannot persist to DB: persistentConversation is nil")
	}

	// 1. Convert a.history ([]*genai.Content) into an []interface{} slice.
	//    The history.Conversation.Messages is already []interface{}, so we just need to assign.
	//    However, history.Append expects individual messages.
	//    Let's clear existing messages in persistentConversation and re-append all.
	//    This ensures the DB state matches the in-memory state.
	agent.persistentConversation.Messages = []*history.Message{} // Clear existing messages
	for _, content := range agent.history {
		// genai.Content is complex. We need to decide what to store.
		// For now, let\'s assume we want to store the serializable representation (e.g., JSON).
		// The history package itself handles the marshaling of messages when Save is called.
		// So, we just need to append the *genai.Content objects directly.
		// Marshal genai.Content to []byte before appending
		contentBytes, err := json.Marshal(content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not marshal genai.Content to save to DB for content: %+v, Error: %v\n", content, err)
			// Decide if we should skip this message or return an error for the whole persistence operation
			continue // Skip this problematic message
		}
		agent.persistentConversation.Append(contentBytes) // Append the marshaled []byte
	}
	agent.trace("PersistToDB", map[string]string{"status": "appending_history_as_bytes", "count": fmt.Sprintf("%d", len(agent.persistentConversation.Messages))})

	// 3. Call history.Save(a.persistentConversation) to save to SQLite.
	err := history.Save(agent.persistentConversation)
	if err != nil {
		// 4. Log any errors from history.Save to os.Stderr and return the error.
		fmt.Fprintf(os.Stderr, "Warning: could not save conversation to DB: %v\n", err)
		agent.trace("PersistToDB", map[string]string{"status": "error", "error": err.Error()})
		return err
	}

	agent.trace("PersistToDB", map[string]string{"status": "success", "conversation_id": agent.persistentConversation.ID})
	return nil
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
