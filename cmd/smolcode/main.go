package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/dhamidi/smolcode"
	"google.golang.org/genai"
)

func main() {
	var conversationPath string
	flag.StringVar(&conversationPath, "conversation", "", "Path to a JSON file to initialize the conversation")
	flag.StringVar(&conversationPath, "c", "", "Path to a JSON file to initialize the conversation (shorthand)")
	flag.Parse()

	initialConversation := smolcode.LoadConversationFromFile(conversationPath)

	smolcode.Code(conversationPath)
}
