package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dhamidi/smolcode"
)

// handleDefaultCommand executes the default smolcode behavior.
// It parses flags relevant to the default operation and calls smolcode.Code.
func handleDefaultCommand(args []string) {
	var specificIDToLoad string
	var continueLatest bool
	var modelName string

	defaultCmd := flag.NewFlagSet("smolcode_default", flag.ExitOnError) // Use a unique name to avoid conflict
	defaultCmd.StringVar(&specificIDToLoad, "conversation-id", "", "ID of a specific conversation to load or continue")
	defaultCmd.StringVar(&specificIDToLoad, "cid", "", "ID of a specific conversation to load or continue (shorthand)")
	defaultCmd.BoolVar(&continueLatest, "continue", false, "Continue the latest conversation")
	defaultCmd.BoolVar(&continueLatest, "c", false, "Continue the latest conversation (shorthand)")
	defaultCmd.StringVar(&modelName, "model", "", "The name of the model to use")
	defaultCmd.StringVar(&modelName, "m", "", "The name of the model to use (shorthand)")

	// Important: Parse only the arguments passed to this handler
	defaultCmd.Parse(args)

	var conversationIDForAgent string
	var forceNewForAgent bool

	if specificIDToLoad != "" {
		conversationIDForAgent = specificIDToLoad
		forceNewForAgent = false
		if continueLatest {
			fmt.Fprintln(os.Stderr, "Warning: Both --continue (-c) and --conversation-id (-cid) were provided. Using the specific ID.")
		}
	} else if continueLatest {
		conversationIDForAgent = ""
		forceNewForAgent = false
	} else {
		conversationIDForAgent = ""
		forceNewForAgent = true
	}

	// Ensure no subcommands like 'plan' are accidentally processed here
	// if they weren't caught by the main dispatcher.
	// This check might be redundant if main dispatcher is robust.
	if defaultCmd.NArg() > 0 {
		argAfterFlags := defaultCmd.Arg(0)
		// List of known top-level commands that should not be processed by default.
		knownCommands := map[string]bool{"plan": true, "memory": true, "history": true, "generate": true}
		if _, isKnownCommand := knownCommands[argAfterFlags]; isKnownCommand {
			// This case should ideally be handled by the main dispatcher.
			// If we reach here, it means os.Args[1] was not a known command,
			// but after parsing flags, a known command appeared as a positional argument.
			// This could happen with `smolcode -cid 123 plan new myplan` if not handled carefully.
			// The original main.go had a check: `if defaultCmd.Arg(0) == "plan"`
			// We should ensure that the main dispatcher prevents this from being routed to default.
			// For safety, one might print a general usage error or a specific one.
			fmt.Fprintf(os.Stderr, "Error: Unexpected subcommand '%s' after flags for default operation.\n", argAfterFlags)
			fmt.Fprintf(os.Stderr, "Usage for default operation: smolcode [flags]\n")
			fmt.Fprintf(os.Stderr, "For subcommands: smolcode <command> [subcommand_args...]\n")
			os.Exit(1)
		}
		// If it's not a known command, it might be an older way of passing instructions,
		// or an unexpected argument. For now, smolcode.Code doesn't take direct instruction args.
		// If there are non-flag arguments, it's likely an error for the default command.
		fmt.Fprintf(os.Stderr, "Error: Unexpected positional arguments for default operation: %v\n", defaultCmd.Args())
		defaultCmd.Usage()
		os.Exit(1)

	}

	if err := smolcode.Code(conversationIDForAgent, modelName, forceNewForAgent); err != nil {
		die("Error running smol-agent: %v", err) // die needs to be accessible
	}
}
