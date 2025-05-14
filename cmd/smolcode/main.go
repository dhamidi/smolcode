package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"smolcode/codegen" // Placeholder: Adjust to your actual module path
)

// stringSliceFlag is a custom flag type to handle multiple occurrences of a string flag
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	// Define a new subcommand: generate
	generateCmd := flag.NewFlagSet("generate", flag.ExitOnError)
	var filePaths stringSliceFlag
	generateCmd.Var(&filePaths, "file", "Path to an existing file (can be specified multiple times)")

	// Check if the first argument is "generate"
	if len(os.Args) < 2 {
		fmt.Println("Usage: smolcode <command> [options]")
		fmt.Println("Available commands: generate")
		// You can add more top-level flags or commands here if needed
		os.Exit(1)
	}

	switch os.Args[1] {
	case "generate":
		// Parse flags specific to the generate command
		if err := generateCmd.Parse(os.Args[2:]); err != nil {
			// Error is already handled by ExitOnError
		}

		args := generateCmd.Args()
		if len(args) == 0 {
			fmt.Println("Usage: smolcode generate --file <path1> --file <path2> ... <instruction>")
			generateCmd.PrintDefaults()
			os.Exit(1)
		}
		instruction := args[0] // The last non-flag argument is the instruction

		handleGenerateCommand(instruction, filePaths)
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		fmt.Println("Available commands: generate")
		os.Exit(1)
	}
}

func handleGenerateCommand(instruction string, filePaths []string) {
	apiKey := os.Getenv("INCEPTION_API_KEY")
	if apiKey == "" {
		log.Fatal("Error: INCEPTION_API_KEY environment variable not set.")
	}

	generator := codegen.New(apiKey)

	var existingFiles []codegen.File
	for _, path := range filePaths {
		content, err := ioutil.ReadFile(path)
		if err != nil {
			log.Printf("Warning: could not read file %s: %v. Skipping.", path, err)
			// Decide if this should be a fatal error or just a warning
			// For now, we'll skip the file and continue
			continue
		}
		existingFiles = append(existingFiles, codegen.File{Path: path, Contents: content})
	}

	fmt.Printf("Instruction: %s\n", instruction)
	if len(existingFiles) > 0 {
		fmt.Println("Provided existing files:")
		for _, f := range existingFiles {
			fmt.Printf("- %s\n", f.Path)
		}
	} else {
		fmt.Println("No existing files provided via --file flags.")
	}

	fmt.Println("Generating code...")
	generatedFiles, err := generator.GenerateCode(instruction, existingFiles)
	if err != nil {
		log.Fatalf("Error generating code: %v", err)
	}

	if len(generatedFiles) == 0 {
		fmt.Println("API returned no files to write.")
		return
	}

	fmt.Println("Writing generated files...")
	err = generator.Write(generatedFiles)
	if err != nil {
		log.Fatalf("Error writing files: %v", err)
	}

	for _, file := range generatedFiles {
		fmt.Printf("Wrote %s\n", file.Path)
	}
	fmt.Println("Code generation complete.")
}
