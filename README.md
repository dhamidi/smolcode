# Smolcode

An agentic experiment: take the [smallest possible coding agent](https://ampcode.com/how-to-build-an-agent), and ask it to modify itself.

What will you get?

Smolcode is my attempt at finding out.

# Usage

The `smolcode` CLI can be invoked in several ways:

1.  **Default Mode (Interactive Coding Agent)**:
    Run `./smolcode` without any subcommands to start the interactive coding agent.
    *   `-c, --conversation <path>`: Optional. Path to a JSON file to initialize the conversation from a previous session.
    *   `-m, --model <model-name>`: Optional. The name of the model to use (e.g., `gemini-1.5-pro-latest`).

2.  **Plan Management**:
    Manage development plans using the `plan` subcommand.
    *   `./smolcode plan new <plan-name>`: Creates a new, empty plan file.
    *   `./smolcode plan inspect <plan-name>`: Displays the plan in Markdown format.
    *   `./smolcode plan next-step <plan-name>`: Displays the next incomplete step of the plan.
    *   `./smolcode plan set <plan-name> <step-id> <status>`: Sets the status of a step. `<status>` can be `DONE` or `TODO`.
    *   `./smolcode plan add-step <plan-name> <step-id> <description> [acceptance-criteria...]`: Adds a new step to the end of the plan. Acceptance criteria are optional.
    *   `./smolcode plan list`: Lists all available plans, showing their status and task counts.
    *   `./smolcode plan reorder <plan-name> <step-id1> [step-id2 ...]`: Reorders steps within a plan. Specified step IDs are moved to the front in the given order; others follow.
    *   `./smolcode plan compact`: Removes all completed plans from storage.
    *   `./smolcode plan remove <plan-name-1> [plan-name-2 ...]`: Removes one or more specified plans from storage.

3.  **Memory Management**:
    Manage the agent's knowledge base using the `memory` subcommand.
    *   `./smolcode memory add <id> <content>`: Adds or updates a memory entry with the given ID and content.
    *   `./smolcode memory get <id>`: Retrieves and displays a memory entry by its ID.
    *   `./smolcode memory search <query>`: Searches memories by a query string and displays matching entries.
    *   `./smolcode memory forget <id>`: Removes a memory entry by its ID.
    *   `./smolcode memory test`: Runs a built-in test to verify memory functionality (add, get, forget). This command will also build the `smolcode` executable.

4.  **Conversation History Management**:
    Manage conversation history using the `history` subcommand.
    *   `./smolcode history new`: Creates a new conversation and saves it.
    *   `./smolcode history append --id <conversation-id> --payload <message-payload>`: Appends a message to an existing conversation.
    *   `./smolcode history list`: Lists all saved conversations with their details.
    *   `./smolcode history show --id <conversation-id>`: Shows the detailed messages of a specific conversation.

# How it works

It's really simple:

- all context is provided in full to the model,
- smolcode offers a growing set of tools,
- we see where the tool calls take us.

Currently smolcode works directly with the Google genai API using `gemini-2.5-pro-05-25`.

# Rules of the game

All changes are made by the agent itself.

All commits are done by the agent itself.

Only when the agent is broken am I allowed to touch code/files.

# Q&A

## Where should I start reading?

1. First, read this blog post: <https://ampcode.com/how-to-build-an-agent> – this agent is based on that
2. Then start reading [agent.go](./agent.go), it contains the meat of the implementation
3. Finally check out individual tools that pique your interest. All tools are implemented in files starting with `tool_`.

## How can I try it out?

Make sure you have your Gemini API key ready.

Then:

```
export GEMINI_API_KEY=add-your-key-here
go build -tags fts5 ./cmd/smolcode
./smolcode
```

If you need fancy line editing:

```
rlwrap ./smolcode
```

The system prompt is read from [smolcode.md](./smolcode.md)
