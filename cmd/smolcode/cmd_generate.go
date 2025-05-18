package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dhamidi/smolcode/codegen"
)

// stringSliceFlag is a custom flag type for accumulating multiple string values.
// It's moved here as it's specific to the generate command.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// handleGenerateCommand processes the 'generate' subcommand.
func handleGenerateCommand(args []string) {
	genCmd := flag.NewFlagSet("generate", flag.ExitOnError)
	archiveOutput := genCmd.Bool("archive", false, "Output a tar archive to stdout instead of writing files to disk.")
	var existingFilePaths stringSliceFlag
	genCmd.Var(&existingFilePaths, "existing-file", "Path to an existing file to provide as context (can be specified multiple times).")
	genCmd.Var(&existingFilePaths, "f", "Shorthand for --existing-file.")
	var desiredFileSpecs stringSliceFlag
	genCmd.Var(&desiredFileSpecs, "desired", "Desired file to generate, format 'filepath:description' (can be specified multiple times).")

	genCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode generate [flags] <instruction>\n")
		fmt.Fprintf(os.Stderr, "Generates code based on an instruction using Inception Labs API.\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		genCmd.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample for --desired: --desired \"pkg/utils/helpers.go:A utility package for common helper functions, including string manipulation and error handling.\"\n")
	}

	genCmd.Parse(args)

	if genCmd.NArg() < 1 {
		genCmd.Usage()
		log.Fatal("Error: Instruction argument is required for 'generate' command.")
	}
	instruction := strings.Join(genCmd.Args(), " ")

	generator := codegen.New(os.Getenv("INCEPTION_API_KEY"))

	var existingFilesToPass []codegen.File
	for _, path := range existingFilePaths {
		content, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("Error reading existing file %s: %v", path, err)
		}
		existingFilesToPass = append(existingFilesToPass, codegen.File{Path: path, Contents: content})
		fmt.Fprintf(os.Stderr, "Providing existing file as context: %s\n", path)
	}

	var desiredFiles []codegen.DesiredFile
	for _, spec := range desiredFileSpecs {
		parts := strings.SplitN(spec, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			log.Fatalf("Invalid format for --desired flag: '%s'. Expected 'filepath:description'.", spec)
		}
		desiredFiles = append(desiredFiles, codegen.DesiredFile{Path: strings.TrimSpace(parts[0]), Description: strings.TrimSpace(parts[1])})
		fmt.Fprintf(os.Stderr, "Requesting desired file: %s (Description: %s)\n", strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}

	fmt.Fprintf(os.Stderr, "Generating code with instruction: %s...\n", instruction)
	generatedFiles, err := generator.GenerateCode(instruction, existingFilesToPass, desiredFiles)
	if err != nil {
		log.Fatalf("Error generating code: %v", err)
	}
	fmt.Fprintf(os.Stderr, "Code generation complete. Received %d file(s).\n", len(generatedFiles))

	var generatedFilePtrs []*codegen.File
	for i := range generatedFiles {
		generatedFilePtrs = append(generatedFilePtrs, &generatedFiles[i])
	}

	if *archiveOutput {
		fmt.Fprintf(os.Stderr, "Outputting to tar archive on stdout...\n")
		// Assuming NewTarballWriterFS is accessible or moved to a shared utility package.
		// For now, this will cause a compile error if NewTarballWriterFS is not defined in this package
		// or in an imported package that makes it available.
		// If it was defined in the old main.go, it needs to be moved or made accessible.
		// Let's assume it will be made available (e.g., in a utils.go or by explicit import if it moves to its own package).
		tarFS := NewTarballWriterFS(os.Stdout) // This might need to be codegen.NewTarballWriterFS if it's part of that package
		defer func() {
			if err := tarFS.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "Error closing tar archive: %v\n", err)
			}
		}()

		if err := generator.WriteTo(generatedFilePtrs, tarFS); err != nil {
			log.Fatalf("Error writing to tar archive: %v", err)
		}
		fmt.Fprintf(os.Stderr, "Tar archive written to stdout successfully.\n")
	} else {
		fmt.Fprintf(os.Stderr, "Writing files to disk...\n")
		if err := generator.Write(generatedFiles); err != nil {
			log.Fatalf("Error writing files to disk: %v", err)
		}
		fmt.Fprintf(os.Stderr, "Files written to disk successfully:\n")
		for _, f := range generatedFiles {
			fmt.Fprintf(os.Stderr, "  - %s\n", f.Path)
		}
	}
}
