package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/dhamidi/smolcode"
	"google.golang.org/genai" // Assuming this path is correct
)

func main() {
	var conversationPath string
	flag.StringVar(&conversationPath, "conversation", "", "Path to a JSON file to initialize the conversation")
	flag.StringVar(&conversationPath, "c", "", "Path to a JSON file to initialize the conversation (shorthand)")
	flag.Parse()

	initialConversation := smolcode.LoadConversationFromFile(conversationPath)

	// TODO: Integrate initialConversation into the smolcode logic.
	//       This will likely involve passing it to the Agent initialization.


	// Call the main agent function (needs modification to accept initialConversation)
	smolcode.Code(conversationPath) // Pass initialConversation here later
}
