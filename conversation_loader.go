package smolcode

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"google.golang.org/genai"
)

// LoadConversationFromFile reads a JSON file from the given path and deserializes
// it into a slice of *genai.Content, representing the initial conversation history.
func LoadConversationFromFile(filepath string) []*genai.Content {
	if filepath == "" {
		return nil // No path provided, return empty history
	}

	// Read the file
	data, err := os.ReadFile(filepath)
	if err != nil {
		log.Printf("Warning: Error reading conversation file %q: %v. Starting with empty history.", filepath, err)
		return nil // Treat file read error as empty history, but log it
	}

	// Deserialize JSON
	var initialConversation []*genai.Content
	if err := json.Unmarshal(data, &initialConversation); err != nil {
		log.Printf("Warning: Error unmarshalling conversation JSON from %q: %v. Starting with empty history.", filepath, err)
		return nil // Treat unmarshal error as empty history, but log it
	}

	fmt.Printf("Loaded %d initial conversation entries from %s\n", len(initialConversation), filepath)
	return initialConversation
}
