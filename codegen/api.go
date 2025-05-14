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

// makeAPIRequest sends a request to the Inceptionlabs API and returns the generated files content.
// For now, `existingFiles` are passed as a simple string representation in the prompt.
// The return type for generated files is a placeholder `[]File` for now.
func makeAPIRequest(apiKey, instruction string, existingFiles []File) ([]File, error) {
	var existingFilesContentBuilder strings.Builder
	if len(existingFiles) > 0 {
		existingFilesContentBuilder.WriteString("\n\nExisting files:\n")
		for _, f := range existingFiles {
			existingFilesContentBuilder.WriteString(fmt.Sprintf("--- %s ---\n%s\n", f.Path, string(f.Contents)))
		}
	}

	userContent := fmt.Sprintf("%s%s", instruction, existingFilesContentBuilder.String())

	reqBody := APIRequest{
		Model: "mercury-coder-small", // As per instruction.md
		Messages: []APIRequestMessage{
			{Role: "system", Content: "You are a helpful assistant that generates code based on instructions and existing files. You should output the generated files in a format that can be parsed, for example, a JSON array of objects, each with a 'path' and 'contents' field."},
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
		return nil, fmt.Errorf("failed to unmarshal API response: %w. Response body: %s", err, string(bodyBytes))
	}

	// --- Placeholder for parsing generated files ---
	// The actual parsing logic will depend on how the API structures the generated files in its response.
	// For now, we expect the API to return a JSON string in the message content that we can then unmarshal into []File.
	var generatedFiles []File
	if len(apiResp.Choices) > 0 && apiResp.Choices[0].Message.Content != "" {
		// Assuming the content of the message is a JSON string representing an array of File objects
		contentStr := apiResp.Choices[0].Message.Content

		// Try to extract content from a markdown JSON block first
		jsonBlockStart := "```json\n"
		jsonBlockEnd := "\n```"
		startIndex := strings.Index(contentStr, jsonBlockStart)
		var extractedJSON string
		if startIndex != -1 {
			endIndex := strings.LastIndex(contentStr, jsonBlockEnd)
			if endIndex != -1 && endIndex > startIndex {
				extractedJSON = contentStr[startIndex+len(jsonBlockStart) : endIndex]
			} else {
				// Malformed markdown block, or end tag missing, try to grab from start of block to end of string
				extractedJSON = contentStr[startIndex+len(jsonBlockStart):]
			}
		} else {
			// Fallback: if no markdown block, try original trimming (though this is less likely to be correct if there was leading text)
			// This case handles if the API returns raw JSON without markdown, or if our markdown check is too simple.
			extractedJSON = strings.TrimSpace(contentStr)
		}

		extractedJSON = strings.TrimSpace(extractedJSON) // Final trim for safety

		if err := json.Unmarshal([]byte(extractedJSON), &generatedFiles); err != nil {
			return nil, fmt.Errorf("failed to unmarshal generated files from API response message content: %w. Extracted JSON string was: %s. Original content: %s", err, extractedJSON, apiResp.Choices[0].Message.Content)
		}
	} else {
		return nil, fmt.Errorf("API response did not contain expected choices or message content. Response body: %s", string(bodyBytes))
	}

	return generatedFiles, nil
}
