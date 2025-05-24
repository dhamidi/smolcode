package smolcode

import "fmt"

// RawTextDisplay is an implementation of TextDisplayer that prints to stdout.
type RawTextDisplay struct{}

// DisplayPrompt prints a formatted prompt string to stdout without a trailing newline.
func (r *RawTextDisplay) DisplayPrompt(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// Display prints the content directly to stdout.
func (r *RawTextDisplay) Display(content string) error {
	fmt.Println(content)
	return nil
}

// DisplayError prints a formatted error message to stdout with red color.
// It mimics the existing agent.errorMessage format.
func (r *RawTextDisplay) DisplayError(format string, args ...interface{}) {
	// We don't have history count here directly, so we'll omit it or pass a placeholder.
	// For simplicity, let's omit it for now, or one could add a way to pass it.
	// The original uses len(agent.history).
	// This is a simplified version.
	fullFormat := fmt.Sprintf("\u001b[91mError\u001b[0m: %s\n", format)
	fmt.Printf(fullFormat, args...)
}

// DisplayMessage prints a formatted message to stdout with a specific role, color, and history count.
// It mimics the existing agent.geminiMessage, agent.toolMessage etc.
func (r *RawTextDisplay) DisplayMessage(role string, colorCode string, historyCount int, format string, args ...interface{}) {
	// Example: \u001b[93mGemini [%d]\u001b[0m: format\n
	// The historyCount might be -1 if not applicable or easily available.
	var rolePrefix string
	if historyCount >= 0 {
		rolePrefix = fmt.Sprintf("\u001b[%sm%s [%%d]\u001b[0m: ", colorCode, role)
		fullFormat := fmt.Sprintf(rolePrefix, historyCount) + format + "\n"
		fmt.Printf(fullFormat, args...)
	} else {
		// Simplified format if history count is not relevant/available
		rolePrefix = fmt.Sprintf("\u001b[%sm%s\u001b[0m: ", colorCode, role)
		fullFormat := rolePrefix + format + "\n"
		fmt.Printf(fullFormat, args...)
	}
}

// TextDisplayer defines an interface for displaying text content.
type TextDisplayer interface {
	Display(content string) error
	DisplayPrompt(format string, args ...interface{}) // For inline prompts
	DisplayError(format string, args ...interface{})
	DisplayMessage(role string, colorCode string, historyCount int, format string, args ...interface{})
}
