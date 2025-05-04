Modify agent.go to:

1. read smolcode.md if it is present in the current directory,
2. use the contents of that file as the system prompt

Pertinent documentation:

```go
package genai // import "google.golang.org/genai"

type GenerateContentConfig struct {
 // Instructions for the model to steer it toward better performance.
 // For example, "Answer as concisely as possible" or "Don't use technical
 // terms in your response".
 SystemInstruction *Content `json:"systemInstruction,omitempty"`
}
    Optional model configuration parameters. For more
    information, see `Content generation parameters
    <https://cloud.google.com/vertex-ai/generative-ai/docs/multimodal/content-generation-parameters>`_.
```

The system prompt should be an additional field of Agent and passed into the constructor in Code().

Start by writing a function that reads the instructions.

Then modify the agent to use the instructions.

Then, when you are done, create a checkpoint.
