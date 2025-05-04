package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// Agent handles communication with the user and the model.
type Agent struct{}

// New creates a new Agent.
func New() *Agent {
	return &Agent{}
}

// ModelMessage represents a message from the model.
type ModelMessage struct {
	Content string `json:"content"`
}

// UserMessage represents a message from the user.
type UserMessage struct {
	Content string `json:"content"`
}

// ToolCallMessage represents a call to a tool.
type ToolCallMessage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

// ToolResultMessage represents the result of a tool call.
type ToolResultMessage struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// ErrorMessage represents an error message.
type ErrorMessage struct {
	Error string `json:"error"`
}

// SendErrorMessage sends an ErrorMessage to stdout.
func (a *Agent) SendErrorMessage(msg ErrorMessage) {
	writeMessage("error", msg)
}

// SendToolResultMessage sends a ToolResultMessage to stdout.
func (a *Agent) SendToolResultMessage(msg ToolResultMessage) {
	writeMessage("tool_result", msg)
}

// SendToolCallMessage sends a ToolCallMessage to stdout.
func (a *Agent) SendToolCallMessage(msg ToolCallMessage) {
	writeMessage("tool_code", msg)
}

// SendUserMessage sends a UserMessage to stdout.
func (a *Agent) SendUserMessage(msg UserMessage) {
	writeMessage("user", msg)
}

// SendModelMessage sends a ModelMessage to stdout.
func (a *Agent) SendModelMessage(msg ModelMessage) {
	writeMessage("model", msg)
}

func writeMessage(messageType string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Fatalf("Error marshalling %s: %v", messageType, err)
	}
	fmt.Fprintf(os.Stdout, "%s: %s\n", messageType, string(jsonData))
}
