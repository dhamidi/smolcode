# Integration Test Design: --continue Conversation Feature

This document outlines the integration test strategy for the `smolcode --continue` feature.

## 1. Setup

The core of the setup involves creating a controlled `history.db` SQLite database with a predefined set of conversations. This can be achieved programmatically using the `history` package functions (`New()`, `Append()`, `SaveTo()`) before test execution.

**Example `history.db` Structure:**

*   **Conversation 1 (`conv_early`)**:
    *   Created: `2023-01-01 10:00:00 UTC`
    *   Messages: ["Hello from early conv"]
*   **Conversation 2 (`conv_late_foo`)**:
    *   Created: `2023-01-01 12:00:00 UTC`
    *   Messages: ["Message 1 from late_foo", "Message 2 from late_foo"]
*   **Conversation 3 (`conv_latest_bar`)**:
    *   Created: `2023-01-01 14:00:00 UTC` (Most recent)
    *   Messages: ["Initial message in latest_bar"]

The test environment should ensure that `SMOLCODE_HISTORY_DB_PATH` (if such an override exists, or by placing the test `history.db` at the default `.smolcode/history.db` relative to the test execution context) points to this controlled database.

## 2. Test Scenarios

The `smolcode` executable will be invoked with different variations of the `--continue` and `--conversation-id` flags.

| Scenario ID | Command                                       | Expected Behavior                                                                 | Verification Notes                                     |
| :---------- | :-------------------------------------------- | :-------------------------------------------------------------------------------- | :----------------------------------------------------- |
| TC1         | `smolcode --continue conv_early`              | Session starts with `conv_early` history.                                         | Check initial prompt/context for "Hello from early conv" |
| TC2         | `smolcode -c conv_late_foo`                   | Session starts with `conv_late_foo` history.                                      | Check for "Message 2 from late_foo"                    |
| TC3         | `smolcode --continue latest`                  | Session starts with `conv_latest_bar` history (most recent).                      | Check for "Initial message in latest_bar"              |
| TC4         | `smolcode --continue`                         | Session starts with `conv_latest_bar` history (flag with no value implies latest).  | Check for "Initial message in latest_bar"              |
| TC5         | `smolcode -c`                                 | Session starts with `conv_latest_bar` history.                                      | Check for "Initial message in latest_bar"              |
| TC6         | `smolcode --conversation-id conv_early`       | Session starts with `conv_early` history.                                         | Check for "Hello from early conv"                      |
| TC7         | `smolcode --cid conv_late_foo`                | Session starts with `conv_late_foo` history.                                      | Check for "Message 2 from late_foo"                    |
| TC8         | `smolcode --cid conv_early --continue latest` | Session starts with `conv_early` (specific ID takes precedence). Prints warning.  | Check for "Hello from early conv", check stderr warning |
| TC9         | `smolcode --continue non_existent_id`         | Session starts as a *new* conversation. Logs error about ID not found.          | Check for new session indication, check stderr/log     |
| TC10        | `smolcode --cid non_existent_id`              | Session starts as a *new* conversation. `smolcode.Code` should handle this.       | Check for new session indication                       |
| TC11        | `smolcode --continue` (with empty DB)         | Session starts as a *new* conversation. Logs "No conversations found".            | Check for new session indication, check stderr/log     |
| TC12        | `smolcode` (no flags, with existing DB)     | Session starts as a *new* conversation.                                         | Check for new session indication                       |

## 3. Verification Strategies

Verifying that the correct conversation history is loaded into the interactive session is the main challenge. Potential methods:

1.  **Output Interception/Parsing:**
    *   If the `smolcode` agent, when starting with history, prints a summary (e.g., "Resuming conversation X with Y messages. Last message: ..."), this output can be captured and asserted. This would be the most straightforward if such output exists or can be added for testing.
2.  **Mocking/Spying on LLM Calls:**
    *   Modify `smolcode.Code` or the underlying LLM interaction layer to allow injection of a mock LLM client during tests.
    *   This mock client can capture the initial prompt/messages sent to the LLM. The test can then assert that these initial messages match the expected conversation history.
    *   This is robust but requires more invasive changes to the core logic for testability.
3.  **Interactive Session Simulation (Limited):**
    *   If the test framework can send input to `smolcode`'s stdin and read from its stdout, a simple initial interaction could be simulated (e.g., send "What was my last message?"). The response would depend on the loaded history. This is complex to make reliable.
4.  **Debug Logging:**
    *   Enhance `smolcode.Code` to include detailed debug logs when a conversation is loaded, indicating the ID and number of messages. Tests can then assert the presence of these log lines.
    *   `log.Printf("Continuing latest conversation: %s", latestID)` is already a good step. Add similar for specific IDs. `smolcode.Code` would need to log what it loads.

**Preferred Strategy:** A combination of (1) if easily available, and (4) by ensuring `smolcode.Code` logs enough information about the conversation it loads (ID, number of messages). If `--verbose` flags or similar can increase log output for tests, that would be beneficial. For warnings (like TC8), checking `stderr` is necessary.

## 4. Test Environment Considerations

*   Ensure `go test` or the integration test runner executes `smolcode` as a separate process to accurately simulate user invocation.
*   Handle cleanup of the test `history.db` after each test or test suite run (e.g., using `t.TempDir()` for database path in test setup).

This plan provides a basis for developing comprehensive integration tests for the conversation continuation feature.
