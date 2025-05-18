package main

import (
	"os"
)

func main() {
	if len(os.Args) < 2 {
		handleDefaultCommand(os.Args[1:]) // Or print usage
		return
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "plan":
		handlePlanCommand(args)
	case "memory":
		handleMemoryCommand(args)
	case "history":
		handleHistoryCommand(args)
	case "generate":
		handleGenerateCommand(args)
	default:
		// If the first arg is not a known command, it might be a flag for the default command,
		// or an unknown command. handleDefaultCommand expects all args including potential flags.
		handleDefaultCommand(os.Args[1:])
	}
}

// die is defined in utils.go

// executeSmolcodeCommand is currently only used by memory test.
// If it were to be used more broadly, it could live in utils.go.
// For now, it's defined in cmd_memory.go as executeSmolcodeCommandInternal
// to avoid conflicts and be specific to its use case.

// TarballWriterFS was previously in main.go and used by handleGenerateCommand.
// It should be moved to a place accessible by cmd_generate.go, e.g., utils.go or codegen package itself.
// For now, cmd_generate.go refers to NewTarballWriterFS, which needs to be defined.
// Let's define it here for now, assuming it might be a general utility.
// Consider moving to codegen package if it's tightly coupled.

// NOTE: The original NewTarballWriterFS was not provided in the initial main.go snippet.
// I will have to assume its previous implementation or simplify. For now, I will
// create a placeholder utils.go for `die` and `NewTarballWriterFS` if I can find its definition.
// The `generate_code` instruction mentioned `NewTarballWriterFS` from `main.go`
// but its code was not in the provided `main.go` content. I'll put `die` here for now.
// `NewTarballWriterFS` and `executeSmolcodeCommand` will be handled in the next step (formatting and fixing).
