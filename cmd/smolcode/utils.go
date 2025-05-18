package main

import (
	"fmt"
	"os"
	"strings"
)

// die prints a formatted error message to stderr and exits with status 1.
func die(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	if !strings.HasSuffix(format, "\n") {
		fmt.Fprintln(os.Stderr) // Add a newline if not already present
	}
	os.Exit(1)
}

// executeSmolcodeCommand - if this is to be a shared utility.
// The version in cmd_memory.go is executeSmolcodeCommandInternal(execPath string, args ...string)
// The original in main.go was func executeSmolcodeCommand(args ...string) (string, error)
// which used os.Args[0]. If a shared version is needed, its signature and use of execPath
// need to be harmonized.
