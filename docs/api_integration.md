# Inceptionlabs API Integration for Code Generation

This document outlines the details of how the `codegen` package interacts with the Inceptionlabs API.

## API Endpoint

All API requests for code generation are made to:

- **URL**: `https://api.inceptionlabs.ai/v1/chat/completions`
- **Method**: `POST`

## Authentication

Requests must include an API key in the `Authorization` header:

- **Header**: `Authorization: Bearer YOUR_INCEPTION_API_KEY`

Replace `YOUR_INCEPTION_API_KEY` with your actual Inceptionlabs API key.

## Request Format

Requests are sent in JSON format with the `Content-Type: application/json` header.

The request body structure is as follows:

```json
{
  "model": "mercury-coder-small",
  "messages": [
    {
      "role": "system", 
      "content": "You are a helpful assistant that generates code based on instructions and existing files. You should output the generated files in a format that can be parsed, for example, a JSON array of objects, each with a 'path' and 'contents' field."
    },
    {
      "role": "user", 
      "content": "<user_instruction>\n\nExisting files:\n--- <file1.path> ---\n<file1.contents>\n--- <file2.path> ---\n<file2.contents>..."
    }
  ]
}
```

- `model`: Specifies the model to be used (e.g., "mercury-coder-small").
- `messages`: An array of message objects.
    - **System Message**: A pre-defined message to set the context for the AI assistant. It instructs the AI on its role and the expected output format for generated files (a JSON string within its content).
    - **User Message**: Contains the user-provided `instruction` and a formatted string representation of any `existingFiles`. The `existingFiles` are concatenated, each with its path and content clearly delineated.

## Response Format

Responses are returned in JSON format.

### Successful Response (2xx Status Code)

A successful response will have a structure similar to this:

```json
{
  "choices": [
    {
      "message": {
        "role": "assistant",
        "content": "```json\n[\n  {\"path\": \"generated_file1.go\", \"contents\": \"cGFja2FnZSBtYWluDQoNCmZtdC5QcmludGxuKFwiaGVsbG8gd29ybGQhXCIpDQo=\"},\n  {\"path\": \"utils/helper.go\", \"contents\": \"cGFja2FnZSB1dGlscw0KDQpmdW5jIEhlbHBGVW5jKCl7fQ0K\"}\n]\n```"
      }
      // ... other choice fields
    }
  ]
  // ... other top-level fields
}
```

- The crucial part is `choices[0].message.content`.
- The `codegen` package expects this `content` field to be a JSON string (potentially wrapped in markdown ```json ... ```) representing an array of `File` objects.
- Each `File` object in the JSON string should have:
    - `path`: The relative path for the generated file (string).
    - `contents`: The content of the file, expected to be a string (which will be converted to `[]byte` in Go). If the content is binary, it should be base64 encoded by the API, and the Go client would decode it. Currently, the `codegen` package assumes it's plain text or already appropriately encoded if the API returns base64 for `[]byte` content type.

### Error Response (4xx or 5xx Status Code)

Error responses include details about the error.

Example error JSON (structure may vary slightly based on error type):
```json
{
  "error": {
    "message": "Incorrect API key provided",
    "type": "auth_error",
    "code": "invalid_api_key"
  }
}
```

The `codegen` package attempts to parse this structure. If parsing fails or the structure differs, it falls back to using the HTTP status code and the raw response body for error reporting.

Common error codes and their meanings (as per `instruction.md`):

- **401**: Incorrect API key provided.
- **429**: Rate limit reached.
- **500**: Server error.
- **503**: Engine overloaded.

## File Content Encoding

The `File` struct in Go uses `[]byte` for `Contents`. When sending existing file contents to the API as part of the user message, they are sent as plain text.

When receiving generated files from the API, the `contents` field within the JSON (in `choices[0].message.content`) is expected to be a string. The Go client currently unmarshals this string directly into `[]byte`. If the API were to send binary content, it would ideally base64 encode it, and the client-side `File` struct or the unmarshalling logic would need to handle base64 decoding into the `[]byte` field. The current implementation assumes the string content is directly convertible/usable as `[]byte` (e.g., UTF-8 text).
