package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/dhamidi/smolcode/memory" // Import the memory package

	"github.com/dhamidi/smolcode"
	"github.com/dhamidi/smolcode/history" // Import the history package
	"github.com/dhamidi/smolcode/planner" // Import the planner package
)

const (
	planStoragePath = ".smolcode/plans.db" // Standardized path
	memoryDBPath    = ".smolcode/memory.db"
)

// executeSmolcodeCommand runs the smolcode CLI with the given arguments.
// It returns the combined stdout/stderr output and any error.
func executeSmolcodeCommand(args ...string) (string, error) {
	executablePath := os.Args[0] // Path to the currently running executable
	cmd := exec.Command(executablePath, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// die prints a formatted error message to stderr and exits with status 1.
func die(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	if !strings.HasSuffix(format, "\n") {
		fmt.Fprintln(os.Stderr) // Add a newline if not already present
	}
	os.Exit(1)
}

func main() {
	// Check if the first argument is "plan"
	if len(os.Args) > 1 && os.Args[1] == "plan" {
		handlePlanCommand(os.Args[2:])
		return // Exit after handling plan command
	}

	// Check if the first argument is "memory"
	if len(os.Args) > 1 && os.Args[1] == "memory" {
		handleMemoryCommand(os.Args[2:])
		return // Exit after handling memory command
	}

	// Check if the first argument is "history"
	if len(os.Args) > 1 && os.Args[1] == "history" {
		handleHistoryCommand(os.Args[2:])
		return // Exit after handling history command
	}

	// Default behavior (original functionality)
	var conversationPath string
	// Need to use a new flag set for the default command to avoid conflicts
	// if we later add flags to the 'plan' subcommand.
	defaultCmd := flag.NewFlagSet("smolcode", flag.ExitOnError)
	defaultCmd.StringVar(&conversationPath, "conversation", "", "Path to a JSON file to initialize the conversation")
	defaultCmd.StringVar(&conversationPath, "c", "", "Path to a JSON file to initialize the conversation (shorthand)")

	var modelName string
	defaultCmd.StringVar(&modelName, "model", "", "The name of the model to use")
	defaultCmd.StringVar(&modelName, "m", "", "The name of the model to use")

	// Parse flags specifically for the default command
	// Note: We parse from os.Args[1:] because os.Args[0] is the program name.
	defaultCmd.Parse(os.Args[1:])

	// Ensure no plan subcommands slipped through if "plan" wasn't the first arg.
	// This prevents `smolcode -c file.json plan new myplan` from working unexpectedly.
	if defaultCmd.Arg(0) == "plan" {
		fmt.Println("Error: 'plan' must be the first argument to use planner subcommands.")
		fmt.Println("Usage: go run cmd/smolcode/main.go plan <subcommand> [arguments]")
		os.Exit(1)
	}

	smolcode.Code(conversationPath, modelName)
}

// handlePlanCommand processes subcommands for the 'plan' feature.
func handlePlanCommand(args []string) {
	plans, err := planner.New(planStoragePath)
	if err != nil {
		log.Fatalf("Error initializing planner: %v", err)
	}

	if len(args) < 1 {
		log.Println("Usage: go run cmd/smolcode/main.go plan <subcommand> [arguments]")
		log.Fatal("Error: No plan subcommand provided.")
	}

	subcommand := args[0]
	remainingArgs := args[1:]

	// Use a dedicated flag set for each subcommand to handle potential future flags
	// and provide better usage messages.
	switch subcommand {
	case "new":
		newCmd := flag.NewFlagSet("new", flag.ExitOnError)
		newCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go plan new <plan-name>\n")
			fmt.Fprintf(os.Stderr, "Creates a new, empty plan file.\n")
		}
		newCmd.Parse(remainingArgs)
		if newCmd.NArg() != 1 {
			newCmd.Usage()
			log.Fatal("Error: 'new' requires exactly one argument: <plan-name>")
		}
		planName := newCmd.Arg(0)
		plan, err := plans.Create(planName)
		if err != nil {
			log.Fatalf("Error creating new plan '%s': %v", planName, err)
		}
		if err := plans.Save(plan); err != nil {
			log.Fatalf("Error saving new plan '%s': %v", planName, err)
		}
		fmt.Printf("Plan '%s' created successfully.\n", planName)

	case "inspect":
		inspectCmd := flag.NewFlagSet("inspect", flag.ExitOnError)
		inspectCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go plan inspect <plan-name>\n")
			fmt.Fprintf(os.Stderr, "Displays the plan in Markdown format.\n")
		}
		inspectCmd.Parse(remainingArgs)
		if inspectCmd.NArg() != 1 {
			inspectCmd.Usage()
			log.Fatal("Error: 'inspect' requires exactly one argument: <plan-name>")
		}
		planName := inspectCmd.Arg(0)
		plan, err := plans.Get(planName)
		if err != nil {
			die("Error loading plan '%s': %v\n", planName, err)
		}
		fmt.Println(plan.Inspect())

	case "next-step":
		nextStepCmd := flag.NewFlagSet("next-step", flag.ExitOnError)
		nextStepCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go plan next-step <plan-name>\n")
			fmt.Fprintf(os.Stderr, "Displays the next incomplete step of the plan.\n")
		}
		nextStepCmd.Parse(remainingArgs)
		if nextStepCmd.NArg() != 1 {
			nextStepCmd.Usage()
			log.Fatal("Error: 'next-step' requires exactly one argument: <plan-name>")
		}
		planName := nextStepCmd.Arg(0)
		plan, err := plans.Get(planName)
		if err != nil {
			die("Error loading plan '%s': %v\n", planName, err)
		}
		next := plan.NextStep()
		if next == nil {
			fmt.Println("Plan is already complete!")
		} else {
			fmt.Printf("Next Step (%s):\n", next.ID())
			fmt.Printf("  Status: %s\n", next.Status())
			fmt.Printf("  Description: %s\n", next.Description())
			if len(next.AcceptanceCriteria()) > 0 {
				fmt.Println("  Acceptance Criteria:")
				for _, crit := range next.AcceptanceCriteria() {
					fmt.Printf("    - %s\n", crit)
				}
			}
		}

	case "set":
		setCmd := flag.NewFlagSet("set", flag.ExitOnError)
		setCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go plan set <plan-name> <step-id> <status>\n")
			fmt.Fprintf(os.Stderr, "Sets the status of a step (DONE or TODO).\n")
		}
		setCmd.Parse(remainingArgs)
		if setCmd.NArg() != 3 {
			setCmd.Usage()
			log.Fatal("Error: 'set' requires exactly three arguments: <plan-name> <step-id> <status>")
		}
		planName := setCmd.Arg(0)
		stepID := setCmd.Arg(1)
		status := strings.ToUpper(setCmd.Arg(2))

		if status != "DONE" && status != "TODO" {
			setCmd.Usage()
			log.Fatalf("Error: Invalid status '%s'. Must be DONE or TODO.", setCmd.Arg(2))
		}

		plan, err := plans.Get(planName)
		if err != nil {
			die("Error loading plan '%s': %v\n", planName, err)
		}

		if status == "DONE" {
			err = plan.MarkAsCompleted(stepID)
		} else {
			err = plan.MarkAsIncomplete(stepID)
		}
		if err != nil {
			log.Fatalf("Error setting status for step '%s' in plan '%s': %v", stepID, planName, err)
		}

		if err := plans.Save(plan); err != nil {
			log.Fatalf("Error saving updated plan '%s': %v", planName, err)
		}
		fmt.Printf("Step '%s' in plan '%s' marked as %s.\n", stepID, planName, status)

	case "add-step":
		addStepCmd := flag.NewFlagSet("add-step", flag.ExitOnError)
		addStepCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go plan add-step <plan-name> <step-id> <description> [acceptance-criteria...]\n")
			fmt.Fprintf(os.Stderr, "Adds a new step to the end of the plan.\n")
		}
		addStepCmd.Parse(remainingArgs)
		if addStepCmd.NArg() < 3 {
			addStepCmd.Usage()
			log.Fatal("Error: 'add-step' requires at least three arguments: <plan-name> <step-id> <description>")
		}
		planName := addStepCmd.Arg(0)
		stepID := addStepCmd.Arg(1)
		description := addStepCmd.Arg(2)
		acceptanceCriteria := addStepCmd.Args()[3:] // Remaining args are criteria

		plan, err := plans.Get(planName)
		if err != nil {
			die("Error loading plan '%s': %v\n", planName, err)
		}

		plan.AddStep(stepID, description, acceptanceCriteria)

		if err := plans.Save(plan); err != nil {
			log.Fatalf("Error saving updated plan '%s': %v", planName, err)
		}
		fmt.Printf("Step '%s' added to plan '%s'.\n", stepID, planName)

	case "list":
		listCmd := flag.NewFlagSet("list", flag.ExitOnError)
		listCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go plan list\n")
			fmt.Fprintf(os.Stderr, "Lists all available plans.\n")
		}
		listCmd.Parse(remainingArgs)
		if listCmd.NArg() != 0 {
			listCmd.Usage()
			log.Fatal("Error: 'list' does not take any arguments")
		}

		planNames, err := plans.List()
		if err != nil {
			log.Fatalf("Error listing plans: %v", err)
		}
		if len(planNames) == 0 {
			fmt.Println("No plans found.")
		} else {
			fmt.Println("Available plans:")
			for _, name := range planNames {
				fmt.Printf("- %s (%s, %d/%d tasks)\n", name.Name, name.Status, name.CompletedTasks, name.TotalTasks)
			}
		}

	case "reorder":
		reorderCmd := flag.NewFlagSet("reorder", flag.ExitOnError)
		reorderCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go plan reorder <plan-name> <step-id1> [step-id2 ...]\n")
			fmt.Fprintf(os.Stderr, "Reorders steps within a plan. Specified step IDs are moved to the front in the given order; others follow.\n")
		}
		reorderCmd.Parse(remainingArgs)
		if reorderCmd.NArg() < 2 { // Must have plan-name and at least one step-id
			reorderCmd.Usage()
			log.Fatal("Error: 'reorder' requires at least two arguments: <plan-name> and <step-id1> [step-id2 ...]")
		}
		planName := reorderCmd.Arg(0)
		newStepOrder := reorderCmd.Args()[1:]

		plan, err := plans.Get(planName)
		if err != nil {
			die("Error loading plan '%s': %v\n", planName, err)
		}

		plan.Reorder(newStepOrder) // Call the in-memory reorder

		if err := plans.Save(plan); err != nil { // Persist the changes
			log.Fatalf("Error saving updated plan '%s' after reordering: %v", planName, err)
		}
		fmt.Printf("Steps in plan '%s' reordered successfully.\n", planName)

	case "compact":
		compactCmd := flag.NewFlagSet("compact", flag.ExitOnError)
		compactCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go plan compact\n")
			fmt.Fprintf(os.Stderr, "Removes all completed plans from storage.\n")
		}
		compactCmd.Parse(remainingArgs)
		if compactCmd.NArg() != 0 {
			compactCmd.Usage()
			log.Fatal("Error: 'compact' does not take any arguments")
		}

		if err := plans.Compact(); err != nil {
			log.Fatalf("Error compacting plans: %v", err)
		}
		fmt.Println("Plans compacted successfully. Completed plans have been removed.")

	case "remove":
		removeCmd := flag.NewFlagSet("remove", flag.ExitOnError)
		removeCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go plan remove <plan-name-1> [plan-name-2 ...]\n")
			fmt.Fprintf(os.Stderr, "Removes one or more specified plans from storage.\n")
		}
		removeCmd.Parse(remainingArgs)
		if removeCmd.NArg() == 0 {
			removeCmd.Usage()
			log.Fatal("Error: 'remove' requires at least one <plan-name> argument")
		}
		planNamesToRemove := removeCmd.Args()
		results := plans.Remove(planNamesToRemove)
		for name, err := range results {
			if err == nil {
				fmt.Printf("Plan '%s' removed successfully.\n", name)
			} else {
				// Check if the error is because the file does not exist
				if os.IsNotExist(err) {
					fmt.Printf("Plan '%s' not found.\n", name)
				} else {
					fmt.Printf("Failed to remove plan '%s': %v\n", name, err)
				}
			}
		}

	default:
		log.Printf("Usage: go run cmd/smolcode/main.go plan <subcommand> [arguments]\n")
		log.Fatalf("Error: Unknown plan subcommand '%s'", subcommand)
	}
}

// handleHistoryCommand processes subcommands for the 'history' feature.
func handleHistoryCommand(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: go run cmd/smolcode/main.go history <subcommand> [arguments]")
		log.Fatal("Error: No history subcommand provided.")
	}

	subcommand := args[0]
	remainingArgs := args[1:] // Will be used later

	switch subcommand {
	case "new":
		newCmd := flag.NewFlagSet("new", flag.ExitOnError)
		newCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go history new\n")
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
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go history append --id <conversation-id> --payload <message-payload>\n")
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

		// For now, we'll assume history.GetByID and history.LoadMessages exist or will be added.
		// This part would need to be adjusted based on the actual history package API for loading.
		// Let's simulate loading, appending, and saving.
		// In a real scenario, history.GetByID would load the conversation.
		// For this step, we'll create a dummy conversation, append to it, and save.
		// This requires a history.Get function. We'll need to define that in history/history.go
		// For now, let's imagine we have a way to load a conversation by ID.
		// To make this runnable without modifying history/history.go *yet*,
		// we'll just create a new conversation and append to it.
		// This isn't the final logic but allows us to structure the CLI.

		// Placeholder for loading the actual conversation by ID
		// conv, err := history.GetByID(conversationID)
		// if err != nil {
		//  log.Fatalf("Error loading conversation '%s': %v", conversationID, err)
		// }
		// For now, let's create a dummy conversation for the sake of CLI structure
		// This will be replaced by actual loading logic later.
		// We need a function to load a conversation.
		// The Save function in history.go overwrites messages.
		// We need a Load function. For now, let's proceed with a conceptual Load.

		// The history package doesn't have a GetByID function.
		// For now, to make progress, I will *temporarily* add a conceptual placeholder
		// that creates a new conversation and appends to it. This is NOT the final
		// implementation for "append" but allows setting up the CLI structure.
		// The plan will need a new step to add GetByID to history/history.go or use an existing one.

		// Let's read the conversation first. We need a function for that.
		// The current history package only has Save and New.
		// It doesn't have a way to load a conversation.
		// This is a problem. I should add a step to the plan to address this.

		conv, err := history.Load(conversationID)
		if err != nil {
			log.Fatalf("Error loading conversation '%s': %v", conversationID, err)
		}

		// The payload from the command line is a string. We need to decide how to store it.
		// For now, we'll store it as a simple string. If complex objects are needed later,
		// this part might need to parse JSON or similar.
		conv.Append(payload) // Append the raw string payload

		if err := history.Save(conv); err != nil {
			log.Fatalf("Error saving updated conversation '%s': %v", conversationID, err)
		}
		fmt.Printf("Message appended to conversation %s successfully.\n", conversationID)

	case "list":
		listCmd := flag.NewFlagSet("list", flag.ExitOnError)
		listCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go history list\n")
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
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go history show --id <conversation-id>\n")
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
			// Attempt to marshal payload to JSON for nice printing. Fallback to %v.
			payloadJSON, jsonErr := json.MarshalIndent(msg.Payload, "      ", "  ")
			if jsonErr != nil {
				fmt.Printf("      Payload: %v\n", msg.Payload)
			} else {
				fmt.Printf("      Payload: %s\n", string(payloadJSON))
			}
		}

	// Add cases for subcommands later
	default:
		fmt.Printf("Usage: go run cmd/smolcode/main.go history <subcommand> [arguments]\n")
		log.Fatalf("Error: Unknown history subcommand '%s'", subcommand)
	}
}

// handleMemoryCommand processes subcommands for the 'memory' feature.
func handleMemoryCommand(args []string) {
	mgr, err := memory.New(memoryDBPath)
	if err != nil {
		log.Fatalf("Error initializing memory manager: %v", err)
	}
	defer func() {
		if err := mgr.Close(); err != nil {
			log.Printf("Error closing memory database: %v", err)
		}
	}()

	if len(args) < 1 {
		log.Println("Usage: go run cmd/smolcode/main.go memory <subcommand> [arguments]")
		log.Fatal("Error: No memory subcommand provided.")
	}

	subcommand := args[0]
	remainingArgs := args[1:]

	switch subcommand {
	case "add":
		addCmd := flag.NewFlagSet("add", flag.ExitOnError)
		addCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go memory add <id> <content>\n")
			fmt.Fprintf(os.Stderr, "Adds or updates a memory.\n")
		}
		addCmd.Parse(remainingArgs)
		if addCmd.NArg() != 2 {
			addCmd.Usage()
			log.Fatal("Error: 'add' requires exactly two arguments: <id> <content>")
		}
		memID := addCmd.Arg(0)
		memContent := addCmd.Arg(1)
		if err := mgr.AddMemory(memID, memContent); err != nil {
			log.Fatalf("Error adding memory '%s': %v", memID, err)
		}
		fmt.Printf("Memory '%s' added/updated successfully.\n", memID)

	case "get":
		getCmd := flag.NewFlagSet("get", flag.ExitOnError)
		getCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go memory get <id>\n")
			fmt.Fprintf(os.Stderr, "Retrieves a memory by its ID.\n")
		}
		getCmd.Parse(remainingArgs)
		if getCmd.NArg() != 1 {
			getCmd.Usage()
			log.Fatal("Error: 'get' requires exactly one argument: <id>")
		}
		memID := getCmd.Arg(0)
		mem, err := mgr.GetMemoryByID(memID)
		if err != nil {
			log.Fatalf("Error retrieving memory '%s': %v", memID, err)
		}
		fmt.Printf("ID: %s\nContent: %s\n", mem.ID, mem.Content)

	case "search":
		searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
		searchCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go memory search <query>\n")
			fmt.Fprintf(os.Stderr, "Searches memories by query.\n")
		}
		searchCmd.Parse(remainingArgs)
		if searchCmd.NArg() != 1 {
			searchCmd.Usage()
			log.Fatal("Error: 'search' requires exactly one argument: <query>")
		}
		query := searchCmd.Arg(0)
		mems, err := mgr.SearchMemory(query)
		if err != nil {
			log.Fatalf("Error searching memory with query '%s': %v", query, err)
		}
		if len(mems) == 0 {
			fmt.Println("No memories found matching your query.")
		} else {
			fmt.Printf("Found %d memory/memories:\n", len(mems))
			for _, mem := range mems {
				fmt.Printf("---\nID: %s\nContent: %s\n", mem.ID, mem.Content)
			}
		}

	case "forget":
		forgetCmd := flag.NewFlagSet("forget", flag.ExitOnError)
		forgetCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go memory forget <id>\n")
			fmt.Fprintf(os.Stderr, "Forgets a memory by its ID.\n")
		}
		forgetCmd.Parse(remainingArgs)
		if forgetCmd.NArg() != 1 {
			forgetCmd.Usage()
			log.Fatal("Error: 'forget' requires exactly one argument: <id>")
		}
		memID := forgetCmd.Arg(0)
		if err := mgr.Forget(memID); err != nil {
			log.Fatalf("Error forgetting memory '%s': %v", memID, err)
		}
		fmt.Printf("Memory '%s' forgotten successfully.\n", memID)

	case "test":
		testCmd := flag.NewFlagSet("test", flag.ExitOnError)
		testCmd.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: go run cmd/smolcode/main.go memory test\n")
			fmt.Fprintf(os.Stderr, "Tests the memory functionality (add, get, forget).\n")
		}
		testCmd.Parse(remainingArgs)
		if testCmd.NArg() != 0 {
			testCmd.Usage()
			log.Fatal("Error: 'test' does not take any arguments")
		}
		fmt.Println("Starting memory test...")

		// Build the smolcode executable to ensure we're testing the latest code
		fmt.Println("Building smolcode executable for test...")
		buildCmd := exec.Command("go", "build", "-tags", "fts5", "-o", "smolcode", "cmd/smolcode/main.go")
		buildOutput, errBuild := buildCmd.CombinedOutput()
		if errBuild != nil {
			log.Fatalf("Failed to build smolcode for test: %v\nOutput: %s", errBuild, string(buildOutput))
		}
		fmt.Println("Build successful.")

		testMemID := "test-mem-cli-refactored"
		testMemContent := "hello from refactored cli test"

		// 1. Add memory
		fmt.Printf("Attempting to add memory: ID='%s', Content='%s'\n", testMemID, testMemContent)
		addOutput, err := executeSmolcodeCommand("memory", "add", testMemID, testMemContent)
		if err != nil {
			log.Fatalf("Failed to add memory: %v. Output: %s", err, addOutput)
		}
		// Check for success message from 'add' command itself, as it doesn't use log.Fatalf on success
		expectedAddSuccess := fmt.Sprintf("Memory '%s' added/updated successfully.\n", testMemID)
		if !strings.Contains(addOutput, expectedAddSuccess) {
			log.Fatalf("Add memory command did not return expected success message. Got: %s", addOutput)
		}
		fmt.Println("Memory added successfully (verified by helper output).")

		// 2. Get memory and verify
		fmt.Printf("Attempting to get memory: ID='%s'\n", testMemID)
		getOutput, err := executeSmolcodeCommand("memory", "get", testMemID)
		if err != nil {
			log.Fatalf("Failed to get memory: %v. Output: %s", err, getOutput)
		}
		expectedGetOutput := fmt.Sprintf("ID: %s\nContent: %s\n", testMemID, testMemContent)
		if !strings.Contains(getOutput, expectedGetOutput) {
			log.Fatalf("Get memory output mismatch. Expected to contain:\n%s\nGot:\n%s", expectedGetOutput, getOutput)
		}
		fmt.Println("Memory retrieved and verified successfully.")

		// 3. Forget memory
		fmt.Printf("Attempting to forget memory: ID='%s'\n", testMemID)
		forgetOutput, err := executeSmolcodeCommand("memory", "forget", testMemID)
		if err != nil {
			log.Fatalf("Failed to forget memory: %v. Output: %s", err, forgetOutput)
		}
		// Check for success message from 'forget' command
		expectedForgetSuccess := fmt.Sprintf("Memory '%s' forgotten successfully.\n", testMemID)
		if !strings.Contains(forgetOutput, expectedForgetSuccess) {
			log.Fatalf("Forget memory command did not return expected success message. Got: %s", forgetOutput)
		}
		fmt.Println("Memory forgotten successfully (verified by helper output).")

		// 4. Get memory again and verify it's not found
		fmt.Printf("Attempting to get memory again (should fail): ID='%s'\n", testMemID)
		getAgainOutput, err := executeSmolcodeCommand("memory", "get", testMemID)

		if err == nil {
			log.Fatalf("Expected an error when trying to get a forgotten memory, but got none. Output: %s", getAgainOutput)
		}
		// The error from executeSmolcodeCommand for a log.Fatalf in the subcommand will be an ExitError.
		// The output string will contain the log.Fatalf message.
		expectedNotFoundMsgPart1 := fmt.Sprintf("Error retrieving memory '%s'", testMemID)           // from log.Fatalf
		expectedNotFoundMsgPart2 := fmt.Sprintf("memory with id '%s': memory: not found", testMemID) // from the error within GetMemoryByID
		if !strings.Contains(getAgainOutput, expectedNotFoundMsgPart1) || !strings.Contains(getAgainOutput, expectedNotFoundMsgPart2) {
			log.Fatalf("Expected 'not found' message for forgotten memory. Got: %s", getAgainOutput)
		}
		fmt.Println("Memory confirmed not found after forgetting.")
		fmt.Println("Memory test completed successfully!")

	default:
		log.Printf("Usage: go run cmd/smolcode/main.go memory <subcommand> [arguments]\n")
		log.Fatalf("Error: Unknown memory subcommand '%s'", subcommand)
	}
}
