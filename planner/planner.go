package planner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings" // Added for strings.Builder
)

// Planner manages plans.
type Planner struct {
	storageDir string // Directory where plans are stored
}

// Plan represents a collection of steps.
type Plan struct {
	ID    string  `json:"id"`    // Unique identifier for the plan, e.g., "active"
	Steps []*Step `json:"steps"` // List of steps in the plan
	name  string  // internal name of the plan, typically the filename without extension
}

// Step represents a single task in a plan.
type Step struct {
	id          string   `json:"id"`          // Short identifier, e.g., "add-tests"
	description string   `json:"description"` // Text description of the step
	status      string   `json:"status"`      // "DONE" or "TODO"
	acceptance  []string `json:"acceptance"`  // List of acceptance criteria
}

// New creates a new Planner instance.
func New(storageDir string) *Planner {
	// TODO: Consider validating the storageDir path or creating it if it doesn't exist.
	return &Planner{
		storageDir: storageDir,
	}
}

// Get retrieves a plan by its name from the storage directory.
// The plan name corresponds to the filename without the .json extension.
func (p *Planner) Get(name string) (*Plan, error) {
	planPath := filepath.Join(p.storageDir, fmt.Sprintf("%s.json", name))
	data, err := os.ReadFile(planPath)
	if err != nil {
		// Handle file not found specifically? Or just return the error?
		// For now, just return the error.
		return nil, fmt.Errorf("failed to read plan file %s: %w", planPath, err)
	}

	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plan %s: %w", name, err)
	}

	// Store the internal name used to load the plan.
	plan.name = name

	return &plan, nil
}

// Inspect converts the plan into a Markdown string representation.
// Each step becomes a headline indicating its status (DONE/TODO).
// The description follows as a paragraph.
// Acceptance criteria are presented as a numbered list.
func (pl *Plan) Inspect() string {
	var builder strings.Builder

	// Maybe add a title for the plan itself?
	// builder.WriteString(fmt.Sprintf("# Plan: %s\\n\\n", pl.ID))

	for i, step := range pl.Steps {
		// Headline includes step number, status, and description (or ID if no description)
		headlineText := step.description
		if headlineText == "" {
			headlineText = step.id // Fallback to ID if description is empty
		}
		// Status is uppercase as per requirement (DONE/TODO)
		builder.WriteString(fmt.Sprintf("## %d. [%s] %s\\n\\n", i+1, strings.ToUpper(step.status), headlineText))

		// Description paragraph (only if description is not empty)
		// if step.description != "" {
		//  builder.WriteString(step.description + "\\n\\n")
		// }
		// The requirement seems to put the description IN the headline, so let's stick to that.

		// Acceptance criteria numbered list
		if len(step.acceptance) > 0 {
			builder.WriteString("Acceptance Criteria:\\n")
			for j, criterion := range step.acceptance {
				builder.WriteString(fmt.Sprintf("%d. %s\\n", j+1, criterion))
			}
			builder.WriteString("\\n") // Add a newline after the list
		}
	}

	return builder.String()
}

// NextStep returns the first step in the plan that is not marked as "DONE".
// It returns nil if all steps are completed.
func (pl *Plan) NextStep() *Step {
	for _, step := range pl.Steps {
		// Case-insensitive comparison just in case
		if strings.ToUpper(step.status) != "DONE" {
			return step
		}
	}
	return nil // All steps are done
}

// ID returns the short identifier of the step.
func (s *Step) ID() string {
	return s.id
}

// Status returns the current status of the step ("DONE" or "TODO").
func (s *Step) Status() string {
	// Ensure status is always returned in uppercase as per requirement.
	return strings.ToUpper(s.status)
}

// Description returns the text description of the step.
func (s *Step) Description() string {
	return s.description
}

// AcceptanceCriteria returns the list of acceptance criteria for the step.
func (s *Step) AcceptanceCriteria() []string {
	// Return a copy to prevent modification of the internal slice? No, requirement is just to return.
	return s.acceptance
}

// MarkAsCompleted finds a step by its ID and sets its status to "DONE".
// Returns an error if the step ID is not found.
func (pl *Plan) MarkAsCompleted(stepID string) error {
	found := false
	for _, step := range pl.Steps {
		if step.id == stepID {
			step.status = "DONE" // Use uppercase consistent with Status() and Inspect()
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("step with ID '%s' not found in plan '%s'", stepID, pl.ID)
	}
	return nil
}

// MarkAsIncomplete finds a step by its ID and sets its status to "TODO".
// Returns an error if the step ID is not found.
func (pl *Plan) MarkAsIncomplete(stepID string) error {
	found := false
	for _, step := range pl.Steps {
		if step.id == stepID {
			step.status = "TODO" // Use uppercase consistent with Status() and Inspect()
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("step with ID '%s' not found in plan '%s'", stepID, pl.ID)
	}
	return nil
}

// IsCompleted checks if all steps in the plan are marked as "DONE".
func (pl *Plan) IsCompleted() bool {
	return pl.NextStep() == nil // If NextStep is nil, all steps are DONE
}

// Save writes the given plan to a JSON file in the planner's storage directory.
// The filename is derived from the plan's internal name field.
func (p *Planner) Save(plan *Plan) error {
	if plan.name == "" {
		// This case should ideally be prevented by how Plan objects are created/retrieved.
		// If ID is guaranteed to be filename-safe and unique, could use that as a fallback.
		return fmt.Errorf("plan has no name, cannot determine save path")
	}
	planPath := filepath.Join(p.storageDir, fmt.Sprintf("%s.json", plan.name))

	// Ensure the storage directory exists. os.MkdirAll is idempotent.
	if err := os.MkdirAll(p.storageDir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory %s: %w", p.storageDir, err)
	}

	data, err := json.MarshalIndent(plan, "", "  ") // Use MarshalIndent for readability
	if err != nil {
		return fmt.Errorf("failed to marshal plan %s for saving: %w", plan.name, err)
	}

	if err := os.WriteFile(planPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write plan file %s: %w", planPath, err)
	}

	return nil
}
