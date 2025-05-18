package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dhamidi/smolcode/planner"
)

const (
	// planStoragePath is specific to the plan command.
	planStoragePath = ".smolcode/plans.db"
)

func handlePlanNewCommand(plans *planner.Planner, args []string) {
	newCmd := flag.NewFlagSet("new", flag.ExitOnError)
	newCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode plan new <plan-name>\n")
		fmt.Fprintf(os.Stderr, "Creates a new, empty plan file.\n")
	}
	newCmd.Parse(args)
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
}

func handlePlanInspectCommand(plans *planner.Planner, args []string) {
	inspectCmd := flag.NewFlagSet("inspect", flag.ExitOnError)
	inspectCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode plan inspect <plan-name>\n")
		fmt.Fprintf(os.Stderr, "Displays the plan in Markdown format.\n")
	}
	inspectCmd.Parse(args)
	if inspectCmd.NArg() != 1 {
		inspectCmd.Usage()
		log.Fatal("Error: 'inspect' requires exactly one argument: <plan-name>")
	}
	planName := inspectCmd.Arg(0)
	plan, err := plans.Get(planName)
	if err != nil {
		die("Error loading plan '%s': %v\n", planName, err) // die needs to be accessible
	}
	fmt.Println(plan.Inspect())
}

func handlePlanNextStepCommand(plans *planner.Planner, args []string) {
	nextStepCmd := flag.NewFlagSet("next-step", flag.ExitOnError)
	nextStepCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode plan next-step <plan-name>\n")
		fmt.Fprintf(os.Stderr, "Displays the next incomplete step of the plan.\n")
	}
	nextStepCmd.Parse(args)
	if nextStepCmd.NArg() != 1 {
		nextStepCmd.Usage()
		log.Fatal("Error: 'next-step' requires exactly one argument: <plan-name>")
	}
	planName := nextStepCmd.Arg(0)
	plan, err := plans.Get(planName)
	if err != nil {
		die("Error loading plan '%s': %v\n", planName, err) // die needs to be accessible
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
}

func handlePlanSetCommand(plans *planner.Planner, args []string) {
	setCmd := flag.NewFlagSet("set", flag.ExitOnError)
	setCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode plan set <plan-name> <step-id> <status>\n")
		fmt.Fprintf(os.Stderr, "Sets the status of a step (DONE or TODO).\n")
	}
	setCmd.Parse(args)
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
		die("Error loading plan '%s': %v\n", planName, err) // die needs to be accessible
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
}

func handlePlanAddStepCommand(plans *planner.Planner, args []string) {
	addStepCmd := flag.NewFlagSet("add-step", flag.ExitOnError)
	addStepCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode plan add-step <plan-name> <step-id> <description> [acceptance-criteria...]\n")
		fmt.Fprintf(os.Stderr, "Adds a new step to the end of the plan.\n")
	}
	addStepCmd.Parse(args)
	if addStepCmd.NArg() < 3 {
		addStepCmd.Usage()
		log.Fatal("Error: 'add-step' requires at least three arguments: <plan-name> <step-id> <description>")
	}
	planName := addStepCmd.Arg(0)
	stepID := addStepCmd.Arg(1)
	description := addStepCmd.Arg(2)
	acceptanceCriteria := addStepCmd.Args()[3:]

	plan, err := plans.Get(planName)
	if err != nil {
		die("Error loading plan '%s': %v\n", planName, err) // die needs to be accessible
	}

	plan.AddStep(stepID, description, acceptanceCriteria)

	if err := plans.Save(plan); err != nil {
		log.Fatalf("Error saving updated plan '%s': %v", planName, err)
	}
	fmt.Printf("Step '%s' added to plan '%s'.\n", stepID, planName)
}

func handlePlanListCommand(plans *planner.Planner, args []string) {
	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	listCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode plan list\n")
		fmt.Fprintf(os.Stderr, "Lists all available plans.\n")
	}
	listCmd.Parse(args)
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
}

func handlePlanReorderCommand(plans *planner.Planner, args []string) {
	reorderCmd := flag.NewFlagSet("reorder", flag.ExitOnError)
	reorderCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode plan reorder <plan-name> <step-id1> [step-id2 ...]\n")
		fmt.Fprintf(os.Stderr, "Reorders steps within a plan. Specified step IDs are moved to the front in the given order; others follow.\n")
	}
	reorderCmd.Parse(args)
	if reorderCmd.NArg() < 2 {
		reorderCmd.Usage()
		log.Fatal("Error: 'reorder' requires at least two arguments: <plan-name> and <step-id1> [step-id2 ...]")
	}
	planName := reorderCmd.Arg(0)
	newStepOrder := reorderCmd.Args()[1:]

	plan, err := plans.Get(planName)
	if err != nil {
		die("Error loading plan '%s': %v\n", planName, err) // die needs to be accessible
	}

	plan.Reorder(newStepOrder)

	if err := plans.Save(plan); err != nil {
		log.Fatalf("Error saving updated plan '%s' after reordering: %v", planName, err)
	}
	fmt.Printf("Steps in plan '%s' reordered successfully.\n", planName)
}

func handlePlanCompactCommand(plans *planner.Planner, args []string) {
	compactCmd := flag.NewFlagSet("compact", flag.ExitOnError)
	compactCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode plan compact\n")
		fmt.Fprintf(os.Stderr, "Removes all completed plans from storage.\n")
	}
	compactCmd.Parse(args)
	if compactCmd.NArg() != 0 {
		compactCmd.Usage()
		log.Fatal("Error: 'compact' does not take any arguments")
	}

	if err := plans.Compact(); err != nil {
		log.Fatalf("Error compacting plans: %v", err)
	}
	fmt.Println("Plans compacted successfully. Completed plans have been removed.")
}

func handlePlanRemoveCommand(plans *planner.Planner, args []string) {
	removeCmd := flag.NewFlagSet("remove", flag.ExitOnError)
	removeCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: smolcode plan remove <plan-name-1> [plan-name-2 ...]\n")
		fmt.Fprintf(os.Stderr, "Removes one or more specified plans from storage.\n")
	}
	removeCmd.Parse(args)
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
			if os.IsNotExist(err) {
				fmt.Printf("Plan '%s' not found.\n", name)
			} else {
				fmt.Printf("Failed to remove plan '%s': %v\n", name, err)
			}
		}
	}
}

// handlePlanCommand processes subcommands for the 'plan' feature.
func handlePlanCommand(args []string) {
	plans, err := planner.New(planStoragePath)
	if err != nil {
		log.Fatalf("Error initializing planner: %v", err)
	}

	if len(args) < 1 {
		log.Println("Usage: smolcode plan <subcommand> [arguments]")
		log.Fatal("Error: No plan subcommand provided.")
	}

	subcommand := args[0]
	remainingArgs := args[1:]

	switch subcommand {
	case "new":
		handlePlanNewCommand(plans, remainingArgs)

	case "inspect":
		handlePlanInspectCommand(plans, remainingArgs)

	case "next-step":
		handlePlanNextStepCommand(plans, remainingArgs)

	case "set":
		handlePlanSetCommand(plans, remainingArgs)

	case "add-step":
		handlePlanAddStepCommand(plans, remainingArgs)

	case "list":
		handlePlanListCommand(plans, remainingArgs)

	case "reorder":
		handlePlanReorderCommand(plans, remainingArgs)

	case "compact":
		handlePlanCompactCommand(plans, remainingArgs)

	case "remove":
		handlePlanRemoveCommand(plans, remainingArgs)

	default:
		log.Printf("Usage: smolcode plan <subcommand> [arguments]\n")
		log.Fatalf("Error: Unknown plan subcommand '%s'", subcommand)
	}
}
