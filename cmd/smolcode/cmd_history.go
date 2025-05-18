package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dhamidi/smolcode/history"
)

// handleHistoryCommand processes subcommands for the 'history' feature.
func handleHistoryCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: smolcode history <subcommand> [arguments]")
		log.Fatal("Error: No history subcommand provided.")
	}

	subcommand := args[0]
	remainingArgs := args[1:]

	switch subcommand {
	case "new":
		newCmd := flag.NewFlagSet("new", flag.ExitOnError)
		newCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: smolcode history new\n")
			fmt.Fprintf(os.Stderr, "Creates a new conversation.\n")
		}
		newCmd.Parse(remainingArgs)
		if newCmd.NArg() != 0 {
			newCmd.Usage()
			log.Fatal("Error: 'new' does not take any arguments")
		}

		conv, err := history.New()
		if err != nil {
			log.Fatalf("Error creating new conversation: %v", err)
		}
		if err := history.Save(conv); err != nil {
			log.Fatalf("Error saving new conversation: %v", err)
		}
		fmt.Printf("New conversation created with ID: %s\n", conv.ID)

	case "append":
		appendCmd := flag.NewFlagSet("append", flag.ExitOnError)
		var conversationID string
		var payload string
		appendCmd.StringVar(&conversationID, "id", "", "ID of the conversation to append to")
		appendCmd.StringVar(&payload, "payload", "", "Payload of the message to append")
		appendCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: smolcode history append --id <conversation-id> --payload <message-payload>\n")
			fmt.Fprintf(os.Stderr, "Appends a message to an existing conversation.\n")
			appendCmd.PrintDefaults()
		}
		appendCmd.Parse(remainingArgs)

		if conversationID == "" {
			appendCmd.Usage()
			log.Fatal("Error: --id flag is required for 'append'")
		}
		if payload == "" {
			appendCmd.Usage()
			log.Fatal("Error: --payload flag is required for 'append'")
		}
		if appendCmd.NArg() != 0 {
			appendCmd.Usage()
			log.Fatal("Error: 'append' does not take positional arguments")
		}

		conv, err := history.Load(conversationID)
		if err != nil {
			log.Fatalf("Error loading conversation '%s': %v", conversationID, err)
		}

		conv.Append(payload) // Append the raw string payload

		if err := history.Save(conv); err != nil {
			log.Fatalf("Error saving updated conversation '%s': %v", conversationID, err)
		}
		fmt.Printf("Message appended to conversation %s successfully.\n", conversationID)

	case "list":
		listCmd := flag.NewFlagSet("list", flag.ExitOnError)
		listCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: smolcode history list\n")
			fmt.Fprintf(os.Stderr, "Lists all conversations.\n")
		}
		listCmd.Parse(remainingArgs)
		if listCmd.NArg() != 0 {
			listCmd.Usage()
			log.Fatal("Error: 'list' does not take any arguments")
		}

		conversations, err := history.ListConversations(history.DefaultDatabasePath)
		if err != nil {
			log.Fatalf("Error listing conversations: %v", err)
		}

		if len(conversations) == 0 {
			fmt.Println("No conversations found.")
		} else {
			fmt.Println("Conversations:")
			for _, conv := range conversations {
				fmt.Printf("  ID: %s, Created: %s, Last Message: %s, Messages: %d\n",
					conv.ID, conv.CreatedAt.Format(time.RFC3339),
					conv.LatestMessageTime.Format(time.RFC3339), conv.MessageCount)
			}
		}

	case "show":
		showCmd := flag.NewFlagSet("show", flag.ExitOnError)
		var conversationID string
		showCmd.StringVar(&conversationID, "id", "", "ID of the conversation to show")
		showCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: smolcode history show --id <conversation-id>\n")
			fmt.Fprintf(os.Stderr, "Shows the details of a specific conversation.\n")
			showCmd.PrintDefaults()
		}
		showCmd.Parse(remainingArgs)

		if conversationID == "" {
			showCmd.Usage()
			log.Fatal("Error: --id flag is required for 'show'")
		}
		if showCmd.NArg() != 0 {
			showCmd.Usage()
			log.Fatal("Error: 'show' does not take positional arguments")
		}

		conv, err := history.Load(conversationID)
		if err != nil {
			log.Fatalf("Error loading conversation '%s': %v", conversationID, err)
		}

		fmt.Printf("Conversation ID: %s\n", conv.ID)
		fmt.Printf("Created At: %s\n", conv.CreatedAt.Format(time.RFC3339))
		fmt.Printf("Messages (%d):\n", len(conv.Messages))
		for i, msg := range conv.Messages {
			fmt.Printf("  [%d] Created At: %s\n", i, msg.CreatedAt.Format(time.RFC3339))
			payloadJSON, jsonErr := json.MarshalIndent(msg.Payload, "      ", "  ")
			if jsonErr != nil {
				fmt.Printf("      Payload: %v\n", msg.Payload)
			} else {
				fmt.Printf("      Payload: %s\n", string(payloadJSON))
			}
		}

	default:
		fmt.Fprintf(os.Stderr, "Usage: smolcode history <subcommand> [arguments]\n")
		log.Fatalf("Error: Unknown history subcommand '%s'", subcommand)
	}
}
