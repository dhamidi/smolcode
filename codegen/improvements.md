# Improving `codegen` for Complex Tasks and Refactoring

This document outlines issues encountered when using the `codegen.Generator.GenerateCode` method (and the associated `./smolcode generate` CLI command) for complex code generation and refactoring tasks, along with potential solutions.

## Problem Description

Attempts to use the `codegen.Generator.GenerateCode` functionality for refactoring `cmd/smolcode/main.go` into multiple subcommand files highlighted significant reliability issues. The core problem lies in the LLM's ability to consistently produce perfectly formatted JSON, especially when the JSON must contain large strings of Go code which include quotes, newlines, and other special characters requiring careful escaping.

**Key Issues Observed:**

1.  **Malformed JSON Output:**
    *   The LLM often produces JSON that is not syntactically correct. Examples include:
        *   Missing quotes around keys (e.g., `	contents` instead of `	"contents"`).
        *   Incorrectly escaped characters within the stringified code content.
        *   Extraneous text or natural language explanations outside the main JSON array/object, even when explicitly prompted against it (e.g., "Here is the file..." preceding the JSON).
        *   Truncated or incomplete JSON structures, especially for larger outputs.
    *   This leads to `json.Unmarshal` errors in `codegen/api.go` when trying to parse the LLM's response into `[]codegen.File` or similar structures.

2.  **Unreliable Adherence to Strict Formatting Prompts:**
    *   Even with highly detailed and strict prompts emphasizing the need for *only* JSON output and precise formatting, the LLM frequently failed to comply.
    *   This suggests that the task of generating complex code, formatting it as a JSON string, and then embedding that string within a larger JSON structure pushes the boundaries of current LLM reliability for this specific API interaction model.

3.  **Single vs. Multiple File Generation:**
    *   Initial attempts to generate all refactored files in a single JSON array (multiple file objects) failed due to malformed JSON.
    *   Subsequent attempts to generate one file at a time (a JSON array with a single file object) also failed due to malformed JSON (e.g., incorrect tabbing before a key, issues with the content string). This indicates the problem isn't just with the number of files, but with the LLM's JSON generation capabilities for this task.

4.  **Impact on Usability for Refactoring:**
    *   These issues make the `generate` command, in its current incarnation, unsuitable for automated, large-scale refactoring tasks that require precise generation of multiple structured code files. The failure rate is too high for practical use.

## Current `codegen/api.go` Parsing Logic

The `makeAPIRequest` function in `codegen/api.go` currently:
1.  Receives the full text response from the LLM.
2.  Attempts to extract a JSON block if the response is wrapped in ```json ... ```.
3.  Tries to unmarshal the (potentially extracted) string into `[]apiGeneratedFile` (an intermediate struct where `Contents` is a string).

This logic is vulnerable if the LLM doesn't use the markdown JSON block or if the JSON within (or without) that block is malformed.

## Potential Solutions and Improvements

1.  **More Robust LLM Output Parsing in `codegen/api.go`:**
    *   **Iterative Repair/Correction:** Instead of a single `json.Unmarshal` attempt, implement a more resilient parser:
        *   If direct unmarshalling fails, attempt common fixes (e.g., try to unescape known problematic sequences, look for and strip common preamble/postamble text like "Here's the JSON:").
        *   Attempt to find *just* the Go code blocks (e.g., using ```go ... ```) within the LLM response if the primary JSON parsing fails for a single file generation task, and then manually construct the file object. This would be a fallback.
    *   **Streaming JSON Parsers:** For very large outputs, a streaming parser might handle partially valid JSON better or identify errors more granularly.
    *   **Contextual Error Reporting:** If parsing fails, provide more context back to the LLM in a (hypothetical) repair loop, showing where the JSON was malformed.

2.  **Change LLM Interaction Model for Code Generation (More Significant Change):**
    *   **Tool-based File Operations:** Instead of asking the LLM to generate a JSON structure containing file paths and full contents, the LLM could be made to call distinct tools for file operations:
        *   `create_file(path, initial_content_prompt)`: LLM decides a file is needed.
        *   `append_to_file(path, content_prompt)`: LLM adds to an existing file.
        *   `replace_in_file(path, old_block_prompt, new_block_prompt)`: For more targeted changes.
        This breaks down the task into smaller, more manageable LLM interactions, where each interaction might produce simpler text rather than complex JSON. This aligns more with how the existing `edit_file` tool works but for generation.
    *   **Dedicated "Refactoring" Tool:** A higher-level tool that takes a list of files and a refactoring instruction. This tool would internally manage multiple LLM calls, perhaps focusing on diffs or specific code transformations rather than full file contents in JSON.

3.  **Prompt Engineering for Simpler Intermediate Formats:**
    *   If direct JSON is too hard, experiment with asking the LLM to produce a sequence of commands or a simpler intermediate representation that a Go program can then parse to create the files. For example:
        ```
        CREATE_FILE: cmd/smolcode/cmd_plan.go
        CONTENT_START:
        package main
        // ... go code ...
        CONTENT_END:
        ---
        CREATE_FILE: cmd/smolcode/cmd_memory.go
        CONTENT_START:
        // ... go code ...
        CONTENT_END:
        ```
        A Go function would then parse this custom format. This shifts complexity from LLM JSON generation to Go parsing logic.

4.  **Client-Side LLM Response Validation and Retry with Feedback:**
    *   If the client (e.g., `./smolcode generate`) receives a response that fails to parse, it could automatically retry the LLM call with an appended message like: "Your previous response was not valid JSON. The error was: <unmarshal_error_details>. Please ensure your entire response is a single, valid JSON array of file objects with no other text." This gives the LLM a chance to self-correct.

5.  **Using `generate_code` for a Single File with Post-Processing:**
    *   When only one file is expected, the prompt could ask for just the raw code content (not wrapped in JSON). The `codegen` package would then wrap this raw content into the `codegen.File` struct itself, assuming a pre-agreed path or deriving it from the prompt. This simplifies the LLM's task significantly by removing the JSON structuring requirement for the content itself.

**Conclusion:**

While the `codegen.Generator.GenerateCode` approach is powerful for simpler, single-file generation tasks where the LLM can produce clean JSON, its current reliability for complex, multi-file, or intricate JSON structures (especially containing escaped code) is low. Improvements likely require more robust parsing on the client-side and/or a shift in the LLM interaction model for these more demanding code generation and refactoring scenarios.
