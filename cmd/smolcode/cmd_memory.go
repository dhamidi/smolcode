package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/dhamidi/smolcode/memory"
)

const (
	// memoryDBPath is specific to the memory command.
	memoryDBPath = ".smolcode/memory.db"
)

func handleMemoryAddCommand(mgr *memory.MemoryManager, args []string) {
	addCmd := flag.NewFlagSet("add", flag.ExitOnError)
	addCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode memory add <id> <content>\n")
		fmt.Fprintf(os.Stderr, "Adds or updates a memory.\n")
	}
	addCmd.Parse(args)
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
}

func handleMemoryGetCommand(mgr *memory.MemoryManager, args []string) {
	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	getCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode memory get <id>\n")
		fmt.Fprintf(os.Stderr, "Retrieves a memory by its ID.\n")
	}
	getCmd.Parse(args)
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
}

func handleMemorySearchCommand(mgr *memory.MemoryManager, args []string) {
	searchCmd := flag.NewFlagSet("search", flag.ExitOnError)
	searchCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode memory search <query>\n")
		fmt.Fprintf(os.Stderr, "Searches memories by query.\n")
	}
	searchCmd.Parse(args)
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
}

func handleMemoryForgetCommand(mgr *memory.MemoryManager, args []string) {
	forgetCmd := flag.NewFlagSet("forget", flag.ExitOnError)
	forgetCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode memory forget <id>\n")
		fmt.Fprintf(os.Stderr, "Forgets a memory by its ID.\n")
	}
	forgetCmd.Parse(args)
	if forgetCmd.NArg() != 1 {
		forgetCmd.Usage()
		log.Fatal("Error: 'forget' requires exactly one argument: <id>")
	}
	memID := forgetCmd.Arg(0)
	if err := mgr.Forget(memID); err != nil {
		log.Fatalf("Error forgetting memory '%s': %v", memID, err)
	}
	fmt.Printf("Memory '%s' forgotten successfully.\n", memID)
}

func handleMemoryTestCommand(args []string) {
	testCmd := flag.NewFlagSet("test", flag.ExitOnError)
	testCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode memory test\n")
		fmt.Fprintf(os.Stderr, "Tests the memory functionality (add, get, forget).\n")
	}
	testCmd.Parse(args)
	if testCmd.NArg() != 0 {
		testCmd.Usage()
		log.Fatal("Error: 'test' does not take any arguments")
	}
	fmt.Println("Starting memory test...")

	fmt.Println("Building smolcode executable for test...")
	// Ensure executeSmolcodeCommand and die are accessible, potentially from a utils package or main
	// For now, assuming they will be made available.
	buildCmd := exec.Command("go", "build", "-tags", "fts5", "-o", "smolcode_test_build", "./cmd/smolcode") // Build from the package dir
	buildOutput, errBuild := buildCmd.CombinedOutput()
	if errBuild != nil {
		log.Fatalf("Failed to build smolcode for test: %v\nOutput: %s", errBuild, string(buildOutput))
	}
	fmt.Println("Build successful.")

	testMemID := "test-mem-cli-refactored"
	testMemContent := "hello from refactored cli test"

	// Adjust executeSmolcodeCommand to use the built test executable
	execPath := "./smolcode_test_build"

	fmt.Printf("Attempting to add memory: ID='%s', Content='%s'\n", testMemID, testMemContent)
	addOutput, err := executeSmolcodeCommandInternal(execPath, "memory", "add", testMemID, testMemContent)
	if err != nil {
		log.Fatalf("Failed to add memory: %v. Output: %s", err, addOutput)
	}
	expectedAddSuccess := fmt.Sprintf("Memory '%s' added/updated successfully.\n", testMemID)
	if !strings.Contains(addOutput, expectedAddSuccess) {
		log.Fatalf("Add memory command did not return expected success message. Got: %s", addOutput)
	}
	fmt.Println("Memory added successfully (verified by helper output).")

	fmt.Printf("Attempting to get memory: ID='%s'\n", testMemID)
	getOutput, err := executeSmolcodeCommandInternal(execPath, "memory", "get", testMemID)
	if err != nil {
		log.Fatalf("Failed to get memory: %v. Output: %s", err, getOutput)
	}
	expectedGetOutput := fmt.Sprintf("ID: %s\nContent: %s\n", testMemID, testMemContent)
	if !strings.Contains(getOutput, expectedGetOutput) {
		log.Fatalf("Get memory output mismatch. Expected to contain:\n%s\nGot:\n%s", expectedGetOutput, getOutput)
	}
	fmt.Println("Memory retrieved and verified successfully.")

	fmt.Printf("Attempting to forget memory: ID='%s'\n", testMemID)
	forgetOutput, err := executeSmolcodeCommandInternal(execPath, "memory", "forget", testMemID)
	if err != nil {
		log.Fatalf("Failed to forget memory: %v. Output: %s", err, forgetOutput)
	}
	expectedForgetSuccess := fmt.Sprintf("Memory '%s' forgotten successfully.\n", testMemID)
	if !strings.Contains(forgetOutput, expectedForgetSuccess) {
		log.Fatalf("Forget memory command did not return expected success message. Got: %s", forgetOutput)
	}
	fmt.Println("Memory forgotten successfully (verified by helper output).")

	fmt.Printf("Attempting to get memory again (should fail): ID='%s'\n", testMemID)
	getAgainOutput, err := executeSmolcodeCommandInternal(execPath, "memory", "get", testMemID)
	if err == nil {
		log.Fatalf("Expected an error when trying to get a forgotten memory, but got none. Output: %s", getAgainOutput)
	}
	expectedNotFoundMsgPart1 := fmt.Sprintf("Error retrieving memory '%s'", testMemID)
	expectedNotFoundMsgPart2 := fmt.Sprintf("memory with id '%s': memory: not found", testMemID)
	if !strings.Contains(getAgainOutput, expectedNotFoundMsgPart1) || !strings.Contains(getAgainOutput, expectedNotFoundMsgPart2) {
		log.Fatalf("Expected 'not found' message for forgotten memory. Got: %s", getAgainOutput)
	}
	fmt.Println("Memory confirmed not found after forgetting.")
	fmt.Println("Memory test completed successfully!")
	// Clean up the test executable
	os.Remove(execPath)
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
		log.Println("Usage: smolcode memory <subcommand> [arguments]")
		log.Fatal("Error: No memory subcommand provided.")
	}

	subcommand := args[0]
	remainingArgs := args[1:]

	switch subcommand {
	case "add":
		handleMemoryAddCommand(mgr, remainingArgs)

	case "get":
		handleMemoryGetCommand(mgr, remainingArgs)

	case "search":
		handleMemorySearchCommand(mgr, remainingArgs)

	case "forget":
		handleMemoryForgetCommand(mgr, remainingArgs)

	case "test":
		handleMemoryTestCommand(remainingArgs)

	default:
		log.Printf("Usage: smolcode memory <subcommand> [arguments]\n")
		log.Fatalf("Error: Unknown memory subcommand '%s'", subcommand)
	}
}

// executeSmolcodeCommandInternal is a helper for the test subcommand to run the built binary.
// It should be similar to the one in main.go but takes the executable path.
func executeSmolcodeCommandInternal(executablePath string, args ...string) (string, error) {
	cmd := exec.Command(executablePath, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}
