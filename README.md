# Smolcode

An agentic experiment: take the [smallest possible coding agent](https://ampcode.com/how-to-build-an-agent), and ask it to modify itself.

What will you get?

Smolcode is my attempt at finding out.

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
