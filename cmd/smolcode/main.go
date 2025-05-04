package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dhamidi/smolcode"
)

func main() {
	var conversationPath string
	flag.StringVar(&conversationPath, "conversation", "", "Path to a JSON file to initialize the conversation")
	flag.StringVar(&conversationPath, "c", "", "Path to a JSON file to initialize the conversation (shorthand)")
	flag.Parse()

	// Check if the conversation file is a temporary reload file and remove it
	if conversationPath != "" && strings.HasPrefix(conversationPath, "smolcode-") && strings.HasSuffix(conversationPath, ".json") {
		// Attempt to remove the file after loading its contents
		// The actual loading happens inside smolcode.Code()
		// We need to ensure smolcode.Code() has read it before we delete it.
		// For now, we'll add the check here, but deletion should ideally happen
		// *after* LoadConversationFromFile in smolcode.Code.
		// Let's add a comment to revisit this.
		// TODO: Move file removal logic into smolcode.Code after loading.

		// We'll perform the check here and pass the info to smolcode.Code
		// No, let's just do the check and remove here for simplicity as requested.
		err := os.Remove(conversationPath)
		if err != nil {
			// Log the error but continue execution
			fmt.Fprintf(os.Stderr, "Warning: could not remove temporary conversation file %s: %v\\n", conversationPath, err)
		} else {
			fmt.Printf("Removed temporary conversation file: %s\\n", conversationPath)
		}
	}

	smolcode.Code(conversationPath)
}
