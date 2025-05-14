package codegen

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMakeAPIRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("Expected to request /v1/chat/completions, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-key" {
			t.Errorf("Expected Authorization header 'Bearer test-key', got %s", authHeader)
		}
		contentTypeHeader := r.Header.Get("Content-Type")
		if contentTypeHeader != "application/json" {
			t.Errorf("Expected Content-Type header 'application/json', got %s", contentTypeHeader)
		}

		// Check request body
		var reqBody APIRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}
		if reqBody.Model != "mercury-coder-small" {
			t.Errorf("Expected model 'mercury-coder-small', got %s", reqBody.Model)
		}
		if len(reqBody.Messages) != 2 {
			t.Fatalf("Expected 2 messages, got %d", len(reqBody.Messages))
		}
		if reqBody.Messages[0].Role != "system" {
			t.Errorf("Expected first message role 'system', got %s", reqBody.Messages[0].Role)
		}
		userMsgContent := "Create a new Go function.\n\nExisting files:\n--- main.go ---\npackage main\nfunc main() {}\n"
		if reqBody.Messages[1].Role != "user" || !strings.Contains(reqBody.Messages[1].Content, "Create a new Go function.") || !strings.Contains(reqBody.Messages[1].Content, userMsgContent) {
			t.Errorf("Unexpected user message content: %s", reqBody.Messages[1].Content)
		}

		// Send response
		respFiles := []File{
			{Path: "new_func.go", Contents: []byte("package main\n\nfunc newFunc() {}")},
		}
		respFilesJSON, _ := json.Marshal(respFiles)
		apiResp := APIResponse{
			Choices: []APIResponseChoice{
				{
					Message: APIRequestMessage{
						Role:    "assistant",
						Content: string(respFilesJSON),
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiResp)
	}))
	defer server.Close()

	// Override the global API URL base to point to the test server
	originalChatEndpoint := chatCompletionsEndpoint               // Store original
	chatCompletionsEndpoint = server.URL + "/v1/chat/completions" // Need to re-assign this as it includes the base
	defer func() {
		chatCompletionsEndpoint = originalChatEndpoint // Restore original
	}()

	existingFiles := []File{
		{Path: "main.go", Contents: []byte("package main\nfunc main() {}")},
	}
	files, err := makeAPIRequest("test-key", "Create a new Go function.", existingFiles)
	if err != nil {
		t.Fatalf("makeAPIRequest failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(files))
	}
	if files[0].Path != "new_func.go" {
		t.Errorf("Expected file path 'new_func.go', got %s", files[0].Path)
	}
	expectedContents := "package main\n\nfunc newFunc() {}"
	if string(files[0].Contents) != expectedContents {
		t.Errorf("Expected file contents %q, got %q", expectedContents, string(files[0].Contents))
	}
}

func TestMakeAPIRequest_SuccessWithLeadingText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Basic validation, more comprehensive checks in TestMakeAPIRequest_Success
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("Expected to request /v1/chat/completions, got %s", r.URL.Path)
		}

		respFiles := []File{
			{Path: "test_output.go", Contents: []byte("package test")},
		}
		respFilesJSON, _ := json.Marshal(respFiles)

		// Simulate leading text before the JSON block
		rawContent := fmt.Sprintf("Here is your generated code:\n```json\n%s\n```\nSome trailing remarks.", string(respFilesJSON))

		apiResp := APIResponse{
			Choices: []APIResponseChoice{
				{
					Message: APIRequestMessage{
						Role:    "assistant",
						Content: rawContent,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiResp)
	}))
	defer server.Close()

	originalChatEndpoint := chatCompletionsEndpoint
	chatCompletionsEndpoint = server.URL + "/v1/chat/completions"
	defer func() {
		chatCompletionsEndpoint = originalChatEndpoint
	}()

	files, err := makeAPIRequest("test-key", "test instruction with leading text", nil)
	if err != nil {
		t.Fatalf("makeAPIRequest failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(files))
	}
	if files[0].Path != "test_output.go" {
		t.Errorf("Expected file path 'test_output.go', got %s", files[0].Path)
	}
	if string(files[0].Contents) != "package test" {
		t.Errorf("Expected file contents %q, got %q", "package test", string(files[0].Contents))
	}
}

func TestMakeAPIRequest_ErrorStatus(t *testing.T) {
	testCases := []struct {
		name        string
		statusCode  int
		errorBody   string
		expectedErr string
	}{
		{
			name:        "401 Unauthorized",
			statusCode:  http.StatusUnauthorized,
			errorBody:   `{"error": {"message": "Incorrect API key", "type": "auth_error", "code": "invalid_api_key"}}`,
			expectedErr: "API error: Incorrect API key (Type: auth_error, Code: invalid_api_key, HTTP Status: 401)",
		},
		{
			name:        "429 Rate Limit",
			statusCode:  http.StatusTooManyRequests,
			errorBody:   `{"error": {"message": "Rate limit exceeded", "type": "rate_limit_error", "code": "rate_limited"}}`,
			expectedErr: "API error: Rate limit exceeded (Type: rate_limit_error, Code: rate_limited, HTTP Status: 429)",
		},
		{
			name:        "500 Server Error",
			statusCode:  http.StatusInternalServerError,
			errorBody:   `{"error": {"message": "Internal server error", "type": "server_error", "code": "internal_error"}}`,
			expectedErr: "API error: Internal server error (Type: server_error, Code: internal_error, HTTP Status: 500)",
		},
		{
			name:        "503 Service Unavailable",
			statusCode:  http.StatusServiceUnavailable,
			errorBody:   `{"error": {"message": "Engine overloaded", "type": "service_error", "code": "engine_overloaded"}}`,
			expectedErr: "API error: Engine overloaded (Type: service_error, Code: engine_overloaded, HTTP Status: 503)",
		},
		{
			name:        "Unknown Error Plain Text",
			statusCode:  http.StatusForbidden,
			errorBody:   "Forbidden",
			expectedErr: "API request failed with status 403: Forbidden",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json") // Even for plain text, API might send this
				w.WriteHeader(tc.statusCode)
				fmt.Fprintln(w, tc.errorBody)
			}))
			defer server.Close()

			originalChatEndpoint := chatCompletionsEndpoint
			chatCompletionsEndpoint = server.URL + "/v1/chat/completions"
			defer func() {
				chatCompletionsEndpoint = originalChatEndpoint
			}()

			_, err := makeAPIRequest("test-key", "test instruction", nil)
			if err == nil {
				t.Fatalf("makeAPIRequest was expected to fail, but it did not")
			}
			if !strings.Contains(err.Error(), tc.expectedErr) {
				t.Errorf("Expected error message to contain %q, got %q", tc.expectedErr, err.Error())
			}
		})
	}
}

func TestMakeAPIRequest_MalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, "not a valid json")
	}))
	defer server.Close()

	originalChatEndpoint := chatCompletionsEndpoint
	chatCompletionsEndpoint = server.URL + "/v1/chat/completions"
	defer func() {
		chatCompletionsEndpoint = originalChatEndpoint
	}()

	_, err := makeAPIRequest("test-key", "test instruction", nil)
	if err == nil {
		t.Fatal("Expected an error due to malformed JSON response, got nil")
	}
	if !strings.Contains(err.Error(), "failed to unmarshal API response") {
		t.Errorf("Expected error about unmarshalling, got: %v", err)
	}
}

func TestMakeAPIRequest_NoChoicesInResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiResp := APIResponse{Choices: []APIResponseChoice{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiResp)
	}))
	defer server.Close()

	originalChatEndpoint := chatCompletionsEndpoint
	chatCompletionsEndpoint = server.URL + "/v1/chat/completions"
	defer func() {
		chatCompletionsEndpoint = originalChatEndpoint
	}()

	_, err := makeAPIRequest("test-key", "test instruction", nil)
	if err == nil {
		t.Fatal("Expected an error due to no choices in response, got nil")
	}
	if !strings.Contains(err.Error(), "API response did not contain expected choices or message content") {
		t.Errorf("Expected error about no choices, got: %v", err)
	}
}

func TestMakeAPIRequest_MalformedFileJSONInMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiResp := APIResponse{
			Choices: []APIResponseChoice{
				{
					Message: APIRequestMessage{
						Role:    "assistant",
						Content: "this is not valid files json",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiResp)
	}))
	defer server.Close()

	originalChatEndpoint := chatCompletionsEndpoint
	chatCompletionsEndpoint = server.URL + "/v1/chat/completions"
	defer func() {
		chatCompletionsEndpoint = originalChatEndpoint
	}()

	_, err := makeAPIRequest("test-key", "test instruction", nil)
	if err == nil {
		t.Fatal("Expected an error due to malformed files JSON in message, got nil")
	}
	if !strings.Contains(err.Error(), "failed to unmarshal generated files from API response message content") {
		t.Errorf("Expected error about unmarshalling files from message, got: %v", err)
	}
}
