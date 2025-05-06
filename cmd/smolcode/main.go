package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/dhamidi/smolcode/memory" // Import the memory package

	"github.com/dhamidi/smolcode"
	"github.com/dhamidi/smolcode/planner" // Import the planner package
)

const (
	planStoragePath = ".smolcode/plans/"
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

	// Default behavior (original functionality)
	var conversationPath string
	// Need to use a new flag set for the default command to avoid conflicts
	// if we later add flags to the 'plan' subcommand.
	defaultCmd := flag.NewFlagSet("smolcode", flag.ExitOnError)
	defaultCmd.StringVar(&conversationPath, "conversation", "", "Path to a JSON file to initialize the conversation")
	defaultCmd.StringVar(&conversationPath, "c", "", "Path to a JSON file to initialize the conversation (shorthand)")

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

	smolcode.Code(conversationPath)
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
		plan := plans.Create(planName)
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
			log.Fatalf("Error loading plan '%s': %v", planName, err)
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
			log.Fatalf("Error loading plan '%s': %v", planName, err)
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
			log.Fatalf("Error loading plan '%s': %v", planName, err)
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
			log.Fatalf("Error loading plan '%s': %v", planName, err)
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
			log.Fatalf("Error loading plan '%s': %v", planName, err)
		}

		plan.Reorder(newStepOrder) // Call the in-memory reorder

		if err := plans.Save(plan); err != nil { // Persist the changes
			log.Fatalf("Error saving updated plan '%s' after reordering: %v", planName, err)
		}
		fmt.Printf("Steps in plan '%s' reordered successfully.\n", planName)

	default:
		log.Printf("Usage: go run cmd/smolcode/main.go plan <subcommand> [arguments]\n")
		log.Fatalf("Error: Unknown plan subcommand '%s'", subcommand)
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
		expectedNotFoundMsgPart1 := fmt.Sprintf("Error retrieving memory '%s'", testMemID)  // from log.Fatalf
		expectedNotFoundMsgPart2 := fmt.Sprintf("memory with id '%s' not found", testMemID) // from the error within GetMemoryByID
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
