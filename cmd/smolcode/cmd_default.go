package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"strings" // Added for parsing MCP flag

	"github.com/dhamidi/smolcode"
	"github.com/dhamidi/smolcode/history"
)

// mcpServerConfigFlag is a custom flag type for parsing MCP server configurations.
type mcpServerConfigFlag []smolcode.MCPServerConfig

func (m *mcpServerConfigFlag) String() string {
	// Convert the slice of MCPServerConfig to a string representation for help messages
	var configs []string
	for _, config := range *m {
		configs = append(configs, fmt.Sprintf("%s:%s", config.ID, config.Command))
	}
	return strings.Join(configs, ", ")
}

func (m *mcpServerConfigFlag) Set(value string) error {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid format for --mcp flag, expected id:command, got: %s", value)
	}
	*m = append(*m, smolcode.MCPServerConfig{ID: parts[0], Command: parts[1]})
	return nil
}

const continueFlagNotSet = "__smolcode_continue_flag_not_set__"

// handleDefaultCommand executes the default smolcode behavior.
// It parses flags relevant to the default operation and calls smolcode.Code.
func handleDefaultCommand(args []string) {
	var specificIDToLoad string
	// var continueLatest bool // Removed
	var continueConvOpt string // Added to accept optional value or "latest"
	var modelName string

	defaultCmd := flag.NewFlagSet("smolcode_default", flag.ExitOnError) // Use a unique name to avoid conflict
	defaultCmd.StringVar(&specificIDToLoad, "conversation-id", "", "ID of a specific conversation to load")
	defaultCmd.StringVar(&specificIDToLoad, "cid", "", "ID of a specific conversation to load (shorthand)")
	defaultCmd.StringVar(&continueConvOpt, "continue", continueFlagNotSet, "Continue a conversation. Provide an ID, 'latest', or pass flag without value to use the latest conversation.")
	defaultCmd.StringVar(&continueConvOpt, "c", continueFlagNotSet, "Continue a conversation. Provide an ID, 'latest', or pass flag without value to use the latest conversation. (shorthand)")
	// Old BoolVar for continue removed
	defaultCmd.StringVar(&modelName, "model", "", "The name of the model to use")
	defaultCmd.StringVar(&modelName, "m", "", "The name of the model to use (shorthand)")

	var mcpConfigs mcpServerConfigFlag
	defaultCmd.Var(&mcpConfigs, "mcp", "Register an MCP server. Format: id:command. Can be used multiple times.")

	// Important: Parse only the arguments passed to this handler
	defaultCmd.Parse(args)

	var conversationIDForAgent string
	var forceNewForAgent bool

	// Determine how to handle conversation loading based on flags
	if specificIDToLoad != "" {
		conversationIDForAgent = specificIDToLoad
		forceNewForAgent = false
		if continueConvOpt != continueFlagNotSet {
			fmt.Fprintln(os.Stderr, "Warning: Both --conversation-id (-cid) and --continue (-c) were provided. Prioritizing --conversation-id.")
		}
	} else if continueConvOpt != continueFlagNotSet { // --continue or -c was used
		if continueConvOpt == "" || continueConvOpt == "latest" { // --continue or --continue=latest
			latestID, err := history.GetLatestConversationID(history.DefaultDatabasePath)
			if err != nil {
				if err == history.ErrConversationNotFound {
					log.Println("No conversations found in history. Starting a new conversation.")
					conversationIDForAgent = ""
					forceNewForAgent = true // No latest, so force new
				} else {
					// For other errors, log it and fall back to a new conversation for robustness.
					log.Printf("Error fetching latest conversation ID: %v. Starting a new conversation.", err)
					conversationIDForAgent = ""
					forceNewForAgent = true
				}
			} else {
				log.Printf("Continuing latest conversation: %s", latestID)
				conversationIDForAgent = latestID
				forceNewForAgent = false
			}
		} else {
			// Request to load specific ID via --continue <id>
			log.Printf("Attempting to continue conversation with ID: %s", continueConvOpt)
			conversationIDForAgent = continueConvOpt
			forceNewForAgent = false
		}
	} else {
		// Default: Start a new conversation
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

	if err := smolcode.Code(conversationIDForAgent, modelName, forceNewForAgent, mcpConfigs); err != nil {
		die("Error running smol-agent: %v", err) // die needs to be accessible
	}
}
