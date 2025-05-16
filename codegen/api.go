package codegen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var (
	apiURLBase              = "https://api.inceptionlabs.ai/v1"
	chatCompletionsEndpoint = apiURLBase + "/chat/completions"
)

// APIRequestMessage represents a message in the API request.

type APIRequestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// APIRequest represents the request body for the chat completions API.

type APIRequest struct {
	Model    string              `json:"model"`
	Messages []APIRequestMessage `json:"messages"`
}

// APIResponseChoice represents a choice in the API response.
// Assuming the response will contain generated code in some structured format.
// This will likely need adjustment based on the actual API response structure for code generation.

type APIResponseChoice struct {
	Message APIRequestMessage `json:"message"`
}

// APIResponse represents the response body from the chat completions API.

type APIResponse struct {
	Choices []APIResponseChoice `json:"choices"`
	Error   *APIErrorDetail     `json:"error,omitempty"` // Custom error field if API returns errors in JSON body
}

// APIErrorDetail represents the structure of an error returned by the API.

type APIErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"` // Can be string or int
}

// makeChatCompletionsRequest sends a request to the Inceptionlabs API for a single file generation.
// It constructs the prompt as per docs.md and returns the deserialized APIResponse.
func makeChatCompletionsRequest(apiKey, instruction string, existingFiles []File, allDesiredFiles []DesiredFile, currentFileToGenerate DesiredFile) (*APIResponse, error) {
	var userMessageBuilder strings.Builder

	// Overall instruction
	userMessageBuilder.WriteString(fmt.Sprintf("Overall instruction:\n%s\n\n", instruction))

	// Existing files
	if len(existingFiles) > 0 {
		userMessageBuilder.WriteString("Existing files (for context):\n")
		for _, f := range existingFiles {
			userMessageBuilder.WriteString(fmt.Sprintf("--- %s ---\n%s\n", f.Path, string(f.Contents)))
		}
		userMessageBuilder.WriteString("\n")
	}

	// List of all desired output files
	if len(allDesiredFiles) > 0 {
		userMessageBuilder.WriteString("Desired output files to be generated:\n")
		for _, df := range allDesiredFiles {
			userMessageBuilder.WriteString(fmt.Sprintf("- %s: %s\n", df.Path, df.Description))
		}
		userMessageBuilder.WriteString("\n")
	}

	// Indication of the currently requested file
	userMessageBuilder.WriteString(fmt.Sprintf("Please generate the content for the following file:\nPath: %s\nDescription: %s\n", currentFileToGenerate.Path, currentFileToGenerate.Description))

	userContent := userMessageBuilder.String()

	reqBody := APIRequest{
		Model: "mercury-coder-small", // As per instruction.md
		Messages: []APIRequestMessage{
			{Role: "system", Content: "You are a helpful assistant that generates code. You will be given an overall instruction, a set of existing reference files, a list of all files to be generated with their descriptions, and the specific file you need to generate now. Your response MUST ONLY be the complete text content for the requested file. Do NOT include any other explanatory text, markdown formatting, or any preamble. Only the raw file content."},
			{Role: "user", Content: userContent},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal API request: %w", err)
	}

	req, err := http.NewRequest("POST", chatCompletionsEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Attempt to parse an error response
		var errResp APIResponse
		parseErr := json.Unmarshal(bodyBytes, &errResp)
		if parseErr == nil && errResp.Error != nil {
			return nil, fmt.Errorf("API error: %s (Type: %s, Code: %v, HTTP Status: %d)", errResp.Error.Message, errResp.Error.Type, errResp.Error.Code, resp.StatusCode)
		}
		// Fallback error message if JSON parsing fails or error structure is different
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var apiResp APIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		// If unmarshalling fails, but there was an API error message in a parsable format in the body, prioritize that.
		var errDetail APIErrorDetail
		if json.Unmarshal(bodyBytes, &errDetail) == nil && errDetail.Message != "" {
			// This is a case where the top-level structure might not match APIResponse (e.g. no 'choices')
			// but an error object is present.
			apiResp.Error = &errDetail
			return &apiResp, fmt.Errorf("API returned an error structure: %s (Type: %s, Code: %v). Original unmarshal error: %w. Response body: %s", errDetail.Message, errDetail.Type, errDetail.Code, err, string(bodyBytes))
		}
		return nil, fmt.Errorf("failed to unmarshal API response: %w. Response body: %s", err, string(bodyBytes))
	}

	// The APIResponse itself (including any error it might contain) is returned directly.
	// The caller (in codegen.go) is responsible for checking apiResp.Error and apiResp.Choices.
	return &apiResp, nil
}
