# Codegen Package: Model Communication Technical Documentation

This document outlines the technical details of how the `codegen` package communicates with the underlying codegen model (Inceptionlabs API) to generate code.

## Overview

The code generation process involves two main Go files: `codegen.go` and `api.go`.

-   `codegen.go`: Provides the primary interface for code generation. It defines the `Generator` type, which orchestrates the process of preparing code generation requests. It is responsible for making multiple API requests (one for each desired file) via Go function calls, managing these calls within a waitgroup, and then writing the received files to disk.
-   `api.go`: Handles the low-level details of interacting with the Inceptionlabs Chat Completions API for a *single file generation request*. Its sole responsibility is to make the HTTP API call with the provided context and return the deserialized HTTP response to the caller. It does not attempt to parse the content of the response beyond basic deserialization.

## Core Components

### `File` Struct (`codegen.go`)

Represents a file to be generated or an existing file provided as context.

```go
type File struct {
    Path     string `json:"path"`
    Contents []byte `json:"contents"`
}
```

-   `Path`: The relative path of the file.
-   `Contents`: The raw byte content of thefile.

### `DesiredFile` Struct (Conceptual - to be implemented in `codegen.go`)

Represents a file the user wants to generate.

```go
// This struct would be defined in codegen.go
type DesiredFile struct {
    Path        string `json:"path"`
    Description string `json:"description"` // Human language description of desired contents
}
```

-   `Path`: The relative path where the generated file should be written.
-   `Description`: A human-readable description of what this file should contain.

### `Generator` Struct (`codegen.go`)

The central component for managing code generation.

```go
type Generator struct {
    apiKey string
}
```

-   `apiKey`: Stores the API key for authenticating with the Inceptionlabs API.

#### Key `Generator` Methods:

-   `New(apiKey string) *Generator`: Constructor to create a new `Generator` instance.
-   `GenerateCode(instruction string, existingFiles []File, desiredOutputFiles []DesiredFile) ([]File, error)`:
    -   Takes a natural language `instruction`, a slice of `existingFiles` (for context), and a slice of `desiredOutputFiles` specifying what to generate.
    -   For each `DesiredFile` in `desiredOutputFiles`:
        -   It will make a separate API request using a Go function call (likely managed by a `sync.WaitGroup` for concurrency).
        -   This request is made by calling a function (e.g., `makeSingleFileAPIRequest` in `api.go`).
        -   The request to `makeSingleFileAPIRequest` will include:
            -   The overall `instruction`.
            -   The full content of all `existingFiles`.
            -   The list of all `desiredOutputFiles` (paths and descriptions).
            -   The specific `DesiredFile` (path and description) currently being generated.
    -   The `makeSingleFileAPIRequest` function (in `api.go`) returns the deserialized API response.
    -   `codegen.go` then takes the raw content from the LLM's response (which should *only* be the file content) and uses it as the `Contents` for the corresponding `File` struct.
    -   Returns a slice of `File` structs representing the generated code and an error if any occurred during the process (e.g., if any of the concurrent API calls fail).
-   `Write(files []File) error` (remains similar):
    -   Takes a slice of `File` structs.
    -   Writes each file to disk at its specified `Path`, overwriting existing files.

## API Interaction for Single File Generation (`api.go`)

The function responsible for a single file generation request (e.g., `makeSingleFileAPIRequest` in `api.go`) handles the communication for one desired output file.

### API Endpoint

-   **Base URL**: `https://api.inceptionlabs.ai/v1`
-   **Chat Completions Endpoint**: `/chat/completions` (full URL: `https://api.inceptionlabs.ai/v1/chat/completions`)

### Request Structure (for a single file generation)

The request to the API is a JSON object with the following key fields:

-   `model` (string): Specifies the model to use (e.g., `"mercury-coder-small"`).
-   `messages` (array of `APIRequestMessage` objects): Defines the conversation context.
    -   `APIRequestMessage`:
        -   `role` (string): Can be "system" or "user".
        -   `content` (string): The message content.

#### Constructing the Request Payload for a Single File:

1.  **System Message**:
    -   Role: `"system"`
    -   Content: **CRITICAL**: `"You are a helpful assistant that generates code. You will be given an overall instruction, a set of existing reference files, a list of all files to be generated with their descriptions, and the specific file you need to generate now. Your response MUST ONLY be the complete text content for the requested file. Do NOT include any other explanatory text, markdown formatting, or any preamble. Only the raw file content."`
    This message primes the model to understand its task and the strict output requirement.

2.  **User Message**:
    -   Role: `"user"`
    -   Content: A carefully constructed prompt including:
        -   The overall `instruction` string.
        -   A formatted block of all `existingFiles` (e.g., `--- path/to/file1.ext ---
content of file1
`).
        -   A list of all `desiredOutputFiles`, including their paths and descriptions (e.g., `Desired output files:
- path/to/output1.ext: Description of output1
- path/to/output2.ext: Description of output2
`).
        -   A clear indication of the *currently requested file* from the `desiredOutputFiles` list, for which the content is being generated in this specific API call (e.g., `Please generate the content for: path/to/output1.ext (Description: Description of output1)`).

### HTTP Request Details

-   **Method**: `POST`
-   **URL**: `https://api.inceptionlabs.ai/v1/chat/completions`
-   **Headers**:
    -   `Authorization`: `Bearer <apiKey>`
    -   `Content-Type`: `application/json`
-   **Body**: The JSON marshaled `APIRequest` struct.

### Response Handling (`api.go`)

1.  **HTTP Call**: `api.go` makes the HTTP POST request.
2.  **Deserialization**: It reads the response body and deserializes it into an `APIResponse` struct (or a similar struct representing the raw API output).
    -   `APIResponse`:
        -   `Choices` (array of `APIResponseChoice`): Contains the model's outputs.
            -   `APIResponseChoice`:
                -   `Message` (`APIRequestMessage`): The actual message from the model. The generated file content is expected *directly* and *only* in `Message.Content`.
        -   `Error` (`*APIErrorDetail`): For API-specific errors.
3.  **Return Value**: `api.go` returns this deserialized `APIResponse` object (or the relevant parts like `Message.Content` and any error) to the caller in `codegen.go`. **`api.go` does NOT parse the `Message.Content` further.**

## Data Flow Summary (New Process)

1.  User calls `generator.GenerateCode(instruction, existingFiles, desiredOutputFiles)`.
2.  `codegen.go` iterates through each `DesiredFile` in `desiredOutputFiles`.
3.  For each `DesiredFile`, `codegen.go` launches a Go routine:
    a.  Inside the Go routine, it calls a function in `api.go` (e.g., `makeSingleFileAPIRequest`).
    b.  This function is passed: the `apiKey`, the overall `instruction`, all `existingFiles`, all `desiredOutputFiles` (paths and descriptions), and the *current* `DesiredFile` (path and description) to be generated.
    c.  `api.go` constructs the specific prompt for this single file, including the critical system message demanding only file content in response.
    d.  `api.go` sends this request as a POST to the Inceptionlabs Chat Completions API.
    e.  `api.go` receives the HTTP response and deserializes it.
    f.  `api.go` returns the deserialized response (specifically, the `Message.Content` which *is* the file content, and any error) to `codegen.go`.
4.  `codegen.go` (in the Go routine) receives the raw file content string. It creates a `codegen.File` struct, setting the `Path` from the `DesiredFile` and `Contents` as `[]byte(rawFileContentString)`.
5.  After all Go routines complete (managed by a waitgroup), `codegen.go` collects all the generated `File` structs.
6.  `GenerateCode` returns the slice of `[]File` to the caller.
7.  The caller can then use `generator.Write(files)` to save the generated files to disk.

## Error Handling Points

-   File writing errors in `Generator.Write`.
-   Errors in `codegen.go` when managing concurrent Go routines or aggregating results.
-   Failures in marshaling the API request in `api.go`.
-   Errors creating the HTTP request object in `api.go`.
-   Errors sending the HTTP request in `api.go`.
-   Errors reading/deserializing the HTTP response body in `api.go`.
-   HTTP status codes >= 400 from the API (handled in `api.go`, propagated to `codegen.go`).
-   Cases where the API response doesn't contain the expected data structure (e.g., missing choices or content), though the primary expectation is that `Message.Content` *is* the file content. If `Message.Content` is missing or empty when a file was expected, this is an error condition.
