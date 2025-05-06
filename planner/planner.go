package planner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Planner manages plans.
type Planner struct {
	storageDir string
}

// Plan represents a collection of steps.
type Plan struct {
	ID    string  `json:"id"` // Unique identifier for the plan, e.g., "active"
	Steps []*Step `json:"steps"`
	name  string  // internal name of the plan, typically the filename without extension
}

// serializablePlan is an internal struct used for JSON marshaling/unmarshaling.
// It has exported fields corresponding to the Plan struct.
type serializablePlan struct {
	ID    string              `json:"id"`
	Steps []*serializableStep `json:"steps"`
}

// Step represents a single task in a plan.
type Step struct {
	id          string   `json:"id"` // Short identifier, e.g., "add-tests"
	description string   `json:"description"`
	status      string   `json:"status"` // "DONE" or "TODO"
	acceptance  []string `json:"acceptance"`
}

// serializableStep is an internal struct used for JSON marshaling/unmarshaling.
// It has exported fields corresponding to the Step struct.
type serializableStep struct {
	Id          string   `json:"id"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Acceptance  []string `json:"acceptance"`
}

// New creates a new Planner instance.
// It ensures the storage directory exists.
// Returns an error if the directory cannot be created.
func New(storageDir string) (*Planner, error) {
	// Ensure the storage directory exists. os.MkdirAll is idempotent.
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory %s: %w", storageDir, err)
	}
	return &Planner{
		storageDir: storageDir,
	}, nil
}

// Create initializes a new Plan in memory with the given name.
// The ID of the plan is set to its name.
// The plan is not persisted until Save is called.
func (p *Planner) Create(name string) *Plan {
	return &Plan{
		ID:    name,
		Steps: []*Step{},
		name:  name, // Also set the internal name for saving
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

	// Unmarshal into the serializable format first
	var sPlan serializablePlan
	if err := json.Unmarshal(data, &sPlan); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plan data %s: %w", name, err)
	}

	// Convert serializablePlan back to Plan
	plan := &Plan{
		ID:    sPlan.ID,
		Steps: make([]*Step, len(sPlan.Steps)),
		name:  name, // Assign internal name from the Get parameter
	}
	for i, sStep := range sPlan.Steps {
		plan.Steps[i] = &Step{
			id:          sStep.Id,
			description: sStep.Description,
			status:      sStep.Status,
			acceptance:  sStep.Acceptance,
		}
	}

	return plan, nil
}

// Inspect converts the plan into a Markdown string representation.
// Each step becomes a headline indicating its status (DONE/TODO).
// The description follows as a paragraph.
// Acceptance criteria are presented as a numbered list.
func (pl *Plan) Inspect() string {
	var builder strings.Builder

	// Maybe add a title for the plan itself?

	for i, step := range pl.Steps {
		// Headline includes step number, status, and description (or ID if no description)
		headlineText := step.description // Use field
		if headlineText == "" {
			headlineText = step.id // Use field
		}
		// Status is uppercase as per requirement (DONE/TODO)
		builder.WriteString(fmt.Sprintf("## %d. [%s] %s\\n\\n", i+1, strings.ToUpper(step.status), headlineText)) // Use field

		// The requirement seems to put the description IN the headline, so let's stick to that.

		if len(step.acceptance) > 0 { // Use field
			builder.WriteString("Acceptance Criteria:\\n")
			for j, criterion := range step.acceptance { // Use field
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
		if strings.ToUpper(step.status) != "DONE" { // Use field
			return step
		}
	}
	return nil // All steps are done
}

// ID returns the short identifier of the step.
func (step *Step) ID() string {
	return step.id
}

// Status returns the current status of the step ("DONE" or "TODO").
func (step *Step) Status() string {
	// Ensure status is always returned in uppercase as per requirement.
	return strings.ToUpper(step.status)
}

// Description returns the text description of the step.
func (step *Step) Description() string {
	return step.description
}

// AcceptanceCriteria returns the list of acceptance criteria for the step.
func (step *Step) AcceptanceCriteria() []string {
	// Return a copy to prevent modification of the internal slice? No, requirement is just to return.
	return step.acceptance
}

// MarkAsCompleted finds a step by its ID and sets its status to "DONE".
// Returns an error if the step ID is not found.
func (pl *Plan) MarkAsCompleted(stepID string) error {
	found := false
	for _, step := range pl.Steps {
		if step.id == stepID { // Use field for comparison
			step.status = "DONE" // Assign to field
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
		if step.id == stepID { // Use field for comparison
			step.status = "TODO" // Assign to field
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("step with ID '%s' not found in plan '%s'", stepID, pl.ID)
	}
	return nil
}

// AddStep appends a new step to the plan.
// The new step is initialized with status "TODO".
func (pl *Plan) AddStep(id, description string, acceptanceCriteria []string) {
	newStep := &Step{
		id:          id,
		description: description,
		status:      "TODO", // Default status for new steps
		acceptance:  acceptanceCriteria,
	}
	pl.Steps = append(pl.Steps, newStep)
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

	// Convert Plan to serializablePlan
	sPlan := serializablePlan{
		ID:    plan.ID,
		Steps: make([]*serializableStep, len(plan.Steps)),
	}
	for i, step := range plan.Steps {
		sPlan.Steps[i] = &serializableStep{
			Id:          step.id,
			Description: step.description,
			Status:      step.status,
			Acceptance:  step.acceptance,
		}
	}

	data, err := json.MarshalIndent(sPlan, "", "  ") // Marshal the serializable version
	if err != nil {
		return fmt.Errorf("failed to marshal plan %s for saving: %w", plan.name, err)
	}

	if err := os.WriteFile(planPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write plan file %s: %w", planPath, err)
	}

	return nil
}
