package main

import (
	"flag"

	"github.com/dhamidi/smolcode"
)

func main() {
	var conversationPath string
	flag.StringVar(&conversationPath, "conversation", "", "Path to a JSON file to initialize the conversation")
	flag.StringVar(&conversationPath, "c", "", "Path to a JSON file to initialize the conversation (shorthand)")
	flag.Parse()

	smolcode.Code(conversationPath)
}
