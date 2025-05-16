package codegen

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMakeChatCompletionsRequest_Success(t *testing.T) {
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

		// Check request body - new prompt structure
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
		// System Message Check (new content)
		expectedSystemMessage := "You are a helpful assistant that generates code. You will be given an overall instruction, a set of existing reference files, a list of all files to be generated with their descriptions, and the specific file you need to generate now. Your response MUST ONLY be the complete text content for the requested file. Do NOT include any other explanatory text, markdown formatting, or any preamble. Only the raw file content."
		if reqBody.Messages[0].Role != "system" || reqBody.Messages[0].Content != expectedSystemMessage {
			t.Errorf("Unexpected system message. Role: %s, Content: %s", reqBody.Messages[0].Role, reqBody.Messages[0].Content)
		}

		// User Message Check (new structure)
		userMsgContent := reqBody.Messages[1].Content
		if reqBody.Messages[1].Role != "user" {
			t.Errorf("Expected second message role 'user', got %s", reqBody.Messages[1].Role)
		}
		if !strings.Contains(userMsgContent, "Overall instruction:\nCreate a new Go function.") {
			t.Errorf("User message missing overall instruction. Got: %s", userMsgContent)
		}
		if !strings.Contains(userMsgContent, "Existing files (for context):\n--- main.go ---\npackage main\nfunc main() {}\n") {
			t.Errorf("User message missing existing files. Got: %s", userMsgContent)
		}
		if !strings.Contains(userMsgContent, "Desired output files to be generated:\n- new_func.go: A new Go function\n- helper.go: A helper utility\n") {
			t.Errorf("User message missing desired output files list. Got: %s", userMsgContent)
		}
		if !strings.Contains(userMsgContent, "Please generate the content for the following file:\nPath: new_func.go\nDescription: A new Go function") {
			t.Errorf("User message missing current file indication. Got: %s", userMsgContent)
		}

		// Send response (raw content, not JSON of files)
		rawGeneratedContent := "package main\n\nfunc newFuncGenerated() {}"
		apiResp := APIResponse{
			Choices: []APIResponseChoice{
				{
					Message: APIRequestMessage{
						Role:    "assistant",
						Content: rawGeneratedContent,
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

	existingFiles := []File{
		{Path: "main.go", Contents: []byte("package main\nfunc main() {}")},
	}
	allDesiredFiles := []DesiredFile{
		{Path: "new_func.go", Description: "A new Go function"},
		{Path: "helper.go", Description: "A helper utility"},
	}
	currentFileToGenerate := DesiredFile{Path: "new_func.go", Description: "A new Go function"}

	resp, err := makeChatCompletionsRequest("test-key", "Create a new Go function.", existingFiles, allDesiredFiles, currentFileToGenerate)
	if err != nil {
		t.Fatalf("makeChatCompletionsRequest failed: %v", err)
	}

	if resp == nil {
		t.Fatalf("Expected a response, got nil")
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(resp.Choices))
	}
	expectedContent := "package main\n\nfunc newFuncGenerated() {}"
	if resp.Choices[0].Message.Content != expectedContent {
		t.Errorf("Expected response content %q, got %q", expectedContent, resp.Choices[0].Message.Content)
	}
}

func TestMakeChatCompletionsRequest_ErrorStatus(t *testing.T) {
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

			_, err := makeChatCompletionsRequest("test-key", "test instruction", nil, nil, DesiredFile{})
			if err == nil {
				t.Fatalf("makeChatCompletionsRequest was expected to fail, but it did not")
			}
			if !strings.Contains(err.Error(), tc.expectedErr) {
				t.Errorf("Expected error message to contain %q, got %q", tc.expectedErr, err.Error())
			}
		})
	}
}

func TestMakeChatCompletionsRequest_MalformedResponse(t *testing.T) {
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

	_, err := makeChatCompletionsRequest("test-key", "test instruction", nil, nil, DesiredFile{})
	if err == nil {
		t.Fatal("Expected an error due to malformed JSON response, got nil")
	}
	if !strings.Contains(err.Error(), "failed to unmarshal API response") {
		t.Errorf("Expected error about unmarshalling, got: %v", err)
	}
}

func TestMakeChatCompletionsRequest_NoChoicesInResponse(t *testing.T) {
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

	apiResp, err := makeChatCompletionsRequest("test-key", "test instruction", nil, nil, DesiredFile{})
	if err != nil { // Should not error at this stage
		t.Fatalf("makeChatCompletionsRequest failed unexpectedly: %v", err)
	}
	if apiResp == nil {
		t.Fatal("Expected a non-nil APIResponse, got nil")
	}
	// The presence of choices is checked by the calling function in codegen.go, not in api.go directly
	// So, here we just check that the call itself was successful.
	if len(apiResp.Choices) != 0 {
		t.Errorf("Expected 0 choices in this test scenario, got %d", len(apiResp.Choices))
	}
}
