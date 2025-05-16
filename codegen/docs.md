# Codegen Package: Model Communication Technical Documentation

This document outlines the technical details of how the `codegen` package communicates with the underlying codegen model (Inceptionlabs API) to generate code.

## Overview

The code generation process involves two main Go files: `codegen.go` and `api.go`.

-   `codegen.go`: Provides the primary interface for code generation. It defines the `Generator` type, which orchestrates the process of sending requests to the API and writing the received files to disk.
-   `api.go`: Handles the low-level details of interacting with the Inceptionlabs Chat Completions API, including request formatting, HTTP communication, and response parsing.

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
-   `Contents`: The raw byte content of the file.

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
-   `GenerateCode(instruction string, existingFiles []File) ([]File, error)`:
    -   Takes a natural language `instruction` and a slice of `existingFiles` (for context).
    -   Calls the internal `makeAPIRequest` function (from `api.go`) to perform the API interaction.
    -   Returns a slice of `File` structs representing the generated code and an error if any occurred.
-   `Write(files []File) error`:
    -   Takes a slice of `File` structs.
    -   Writes each file to disk at its specified `Path`, overwriting existing files.

## API Interaction (`api.go`)

The `makeAPIRequest` function in `api.go` is responsible for the entire communication lifecycle with the Inceptionlabs API.

### API Endpoint

-   **Base URL**: `https://api.inceptionlabs.ai/v1`
-   **Chat Completions Endpoint**: `/chat/completions` (full URL: `https://api.inceptionlabs.ai/v1/chat/completions`)

### Request Structure

The request to the API is a JSON object with the following key fields:

-   `model` (string): Specifies the model to use. Currently hardcoded to `"mercury-coder-small"`.
-   `messages` (array of `APIRequestMessage` objects): Defines the conversation context.
    -   `APIRequestMessage`:
        -   `role` (string): Can be "system" or "user".
        -   `content` (string): The message content.

#### Constructing the Request Payload:

1.  **System Message**:
    -   Role: `"system"`
    -   Content: `"You are a helpful assistant that generates code based on instructions and existing files. You should output the generated files in a format that can be parsed, for example, a JSON array of objects, each with a 'path' and 'contents' field."`
    This message primes the model to understand its task and the expected output format for generated files.

2.  **User Message**:
    -   Role: `"user"`
    -   Content: A concatenation of:
        -   The `instruction` string provided to `GenerateCode`.
        -   A formatted block of `existingFiles` (if any), presented as:
            ```
            Existing files:
            --- path/to/file1.ext ---
            content of file1
            --- path/to/file2.ext ---
            content of file2
            ```

### HTTP Request Details

-   **Method**: `POST`
-   **URL**: `https://api.inceptionlabs.ai/v1/chat/completions`
-   **Headers**:
    -   `Authorization`: `Bearer <apiKey>`
    -   `Content-Type`: `application/json`
-   **Body**: The JSON marshaled `APIRequest` struct.

### Response Handling

1.  **Error Checking**:
    -   The HTTP response status code is checked. If >= 400, it's treated as an error.
    -   The system attempts to parse a JSON error structure (`APIErrorDetail`) from the response body if the API returns errors in that format. Otherwise, a generic error message with the status code and response body is returned.
    -   `APIErrorDetail`: Contains `Message`, `Type`, and `Code` fields.

2.  **Successful Response Parsing**:
    -   The response body is unmarshaled into an `APIResponse` struct.
    -   `APIResponse`:
        -   `Choices` (array of `APIResponseChoice`): Contains the model's outputs.
            -   `APIResponseChoice`:
                -   `Message` (`APIRequestMessage`): The actual message from the model. The generated file data is expected within `Message.Content`.
        -   `Error` (`*APIErrorDetail`): For API-specific errors returned within a 2xx response (less common).

3.  **Extracting Generated Files**:
    -   The content is expected to be in `APIResponse.Choices[0].Message.Content`.
    -   This `Content` string is assumed to be a JSON array of file objects.
    -   **Extraction Logic**:
        1.  The code first attempts to find and extract JSON content from a markdown code block (e.g., ```json ... ```). This handles cases where the API might wrap its JSON output.
        2.  If a markdown block isn't found, it falls back to trimming whitespace from the `Content` string, assuming it might be raw JSON.
    -   **Unmarshaling Files**:
        1.  An intermediate struct `apiGeneratedFile` is used for unmarshaling, where `Contents` is a `string`:
            ```go
            type apiGeneratedFile struct {
                Path     string
                Contents string
            }
            ```
        2.  The extracted (and potentially cleaned) JSON string is unmarshaled into `[]apiGeneratedFile`.
        3.  This `[]apiGeneratedFile` slice is then converted into the `[]codegen.File` slice, where `Contents` (string) is converted to `[]byte`.
    -   If no choices or message content are found, an error is returned.

## Data Flow Summary

1.  User calls `generator.GenerateCode(instruction, existingFiles)`.
2.  `GenerateCode` calls `makeAPIRequest(apiKey, instruction, existingFiles)`.
3.  `makeAPIRequest`:
    a.  Formats `instruction` and `existingFiles` into a user prompt string.
    b.  Constructs an `APIRequest` with this prompt and a system message defining the expected JSON output format for files.
    c.  Sends this request as a POST to the Inceptionlabs Chat Completions API.
    d.  Receives the HTTP response.
    e.  Parses the response, expecting a JSON string within the assistant's message content.
    f.  This JSON string should represent an array of objects, each with `path` and `contents` (string) fields.
    g.  Unmarshals this JSON into `[]apiGeneratedFile`.
    h.  Converts `[]apiGeneratedFile` to `[]codegen.File` (converting string contents to `[]byte`).
    i.  Returns `[]codegen.File` to `GenerateCode`.
4.  `GenerateCode` returns the `[]File` to the caller.
5.  The caller can then use `generator.Write(files)` to save the generated files to disk.

## Error Handling Points

-   File writing errors in `Generator.Write`.
-   Failures in marshaling the API request in `makeAPIRequest`.
-   Errors creating the HTTP request object in `makeAPIRequest`.
-   Errors sending the HTTP request in `makeAPIRequest`.
-   Errors reading the HTTP response body in `makeAPIRequest`.
-   HTTP status codes >= 400 from the API.
-   Errors unmarshaling the API JSON response (both error and success cases).
-   Errors unmarshaling the generated file data from the API response message content.
-   Cases where the API response doesn't contain the expected data structure (e.g., missing choices or content).
