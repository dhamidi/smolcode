package smolcode

import (
	"fmt"
	"os" // Added for Fprintf to os.Stderr

	"github.com/charmbracelet/glamour"
)

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

// GlamourousTextDisplay attempts to render text using glamour, falling back to RawTextDisplay.
type GlamourousTextDisplay struct {
	RawTextDisplay // Embed RawTextDisplay for fallback and to satisfy the interface for non-glamour methods.
}

// Display attempts to render the content using glamour. If it fails, it falls back to RawTextDisplay.
func (g *GlamourousTextDisplay) Display(content string) error {
	prettyOutput, err := glamour.RenderWithEnvironmentConfig(content)
	if err != nil {
		// Fallback to RawTextDisplay's Display method
		fmt.Fprintf(os.Stderr, "Glamour rendering failed: %v. Falling back to raw display.\n", err) // It's good to inform about the fallback
		return g.RawTextDisplay.Display(content)
	}
	fmt.Println(prettyOutput)
	return nil
}

// DisplayPrompt delegates directly to RawTextDisplay as glamour is not typically desired for prompts.
func (g *GlamourousTextDisplay) DisplayPrompt(format string, args ...interface{}) {
	g.RawTextDisplay.DisplayPrompt(format, args...)
}

// DisplayError delegates directly to RawTextDisplay for consistent error formatting.
func (g *GlamourousTextDisplay) DisplayError(format string, args ...interface{}) {
	g.RawTextDisplay.DisplayError(format, args...)
}

// DisplayMessage attempts to render the main message content using glamour.
// The role prefix is printed raw, then the glamour-rendered message.
// Falls back to RawTextDisplay.DisplayMessage if glamour rendering fails.
func (g *GlamourousTextDisplay) DisplayMessage(role string, colorCode string, historyCount int, format string, args ...interface{}) {
	coreMessage := fmt.Sprintf(format, args...)

	// Attempt to render the core message with glamour
	prettyOutput, err := glamour.RenderWithEnvironmentConfig(coreMessage)
	if err != nil {
		// Fallback to RawTextDisplay's DisplayMessage method for the whole message
		fmt.Fprintf(os.Stderr, "Glamour rendering failed for message: %v. Falling back to raw display for the entire message.\n", err)
		g.RawTextDisplay.DisplayMessage(role, colorCode, historyCount, format, args...) // Pass original format and args
		return
	}

	// Print the role prefix directly (raw)
	var rolePrefix string
	if historyCount >= 0 {
		rolePrefix = fmt.Sprintf("\u001b[%sm%s [%d]\u001b[0m: ", colorCode, role, historyCount)
	} else {
		rolePrefix = fmt.Sprintf("\u001b[%sm%s\u001b[0m: ", colorCode, role)
	}
	fmt.Printf(rolePrefix) // Print prefix without newline

	// Print the glamour-rendered message (which usually includes its own newline handling)
	fmt.Println(prettyOutput)
}
