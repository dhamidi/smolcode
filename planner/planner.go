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

// PlanInfo holds summary information about a plan.
// This is used by the List method.
type PlanInfo struct {
	Name           string `json:"name"`
	Status         string `json:"status"` // "DONE" or "TODO"
	TotalTasks     int    `json:"total_tasks"`
	CompletedTasks int    `json:"completed_tasks"`
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

func (pl *Plan) Inspect() string {
	var builder strings.Builder

	// Maybe add a title for the plan itself?
	// builder.WriteString(fmt.Sprintf("# Plan: %s\n\n", pl.ID))

	for i, step := range pl.Steps {
		// Headline: includes step number, status, and ID.
		header := fmt.Sprintf("## %d. [%s] %s\n", i+1, strings.ToUpper(step.status), step.id) // Use fields
		builder.WriteString(header)

		// Description paragraph (if not empty)
		if step.description != "" {
			builder.WriteString("\n" + step.description + "\n") // Add blank lines around description
		}
		builder.WriteString("\n") // Ensure a blank line after header or description

		// Acceptance criteria numbered list
		if len(step.acceptance) > 0 { // Use field
			builder.WriteString("Acceptance Criteria:\n")
			for j, criterion := range step.acceptance { // Use field
				builder.WriteString(fmt.Sprintf("%d. %s\n", j+1, criterion))
			}
			builder.WriteString("\n") // Add a newline after the list
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

// Compact removes all completed plans from the storage directory.
// A plan is considered completed if all its steps are marked as "DONE".
func (p *Planner) Compact() error {
	files, err := os.ReadDir(p.storageDir)
	if err != nil {
		return fmt.Errorf("failed to read plan storage directory %s for compacting: %w", p.storageDir, err)
	}

	var compactedCount int // To keep track of how many plans are removed.
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			planName := strings.TrimSuffix(file.Name(), ".json")
			plan, err := p.Get(planName) // Use existing Get method to load plan details
			if err != nil {
				// Log error and continue, so a single corrupted/unreadable plan doesn't stop compaction.
				fmt.Fprintf(os.Stderr, "warning: failed to load plan '%s' during compact: %v\n", planName, err)
				continue
			}

			if plan.IsCompleted() { // Use existing IsCompleted method
				planPath := filepath.Join(p.storageDir, file.Name())
				if err := os.Remove(planPath); err != nil {
					// Log error and continue. Perhaps the file was removed by another process or permissions issue.
					fmt.Fprintf(os.Stderr, "warning: failed to remove completed plan file '%s': %v\n", planPath, err)
					continue
				}
				compactedCount++
				// Optional: Log successful removal for verbosity if desired.
				// fmt.Printf("Compacted (removed) completed plan: %s\n", planName)
			}
		}
	}

	// Optional: Log overall result for verbosity if desired.
	// fmt.Printf("Compaction complete. Removed %d completed plan(s).\n", compactedCount)
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

// Compact removes all completed plans from the storage directory.
// A plan is considered completed if all its steps are marked as "DONE".
func (p *Planner) Compact() error {
	files, err := os.ReadDir(p.storageDir)
	if err != nil {
		return fmt.Errorf("failed to read plan storage directory %s for compacting: %w", p.storageDir, err)
	}

	var compactedCount int // To keep track of how many plans are removed.
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			planName := strings.TrimSuffix(file.Name(), ".json")
			plan, err := p.Get(planName) // Use existing Get method to load plan details
			if err != nil {
				// Log error and continue, so a single corrupted/unreadable plan doesn't stop compaction.
				fmt.Fprintf(os.Stderr, "warning: failed to load plan '%s' during compact: %v\n", planName, err)
				continue
			}

			if plan.IsCompleted() { // Use existing IsCompleted method
				planPath := filepath.Join(p.storageDir, file.Name())
				if err := os.Remove(planPath); err != nil {
					// Log error and continue. Perhaps the file was removed by another process or permissions issue.
					fmt.Fprintf(os.Stderr, "warning: failed to remove completed plan file '%s': %v\n", planPath, err)
					continue
				}
				compactedCount++
				// Optional: Log successful removal for verbosity if desired.
				// fmt.Printf("Compacted (removed) completed plan: %s\n", planName)
			}
		}
	}

	// Optional: Log overall result for verbosity if desired.
	// fmt.Printf("Compaction complete. Removed %d completed plan(s).\n", compactedCount)
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

// List returns summary information for all plans in the storage directory.
func (p *Planner) List() ([]PlanInfo, error) {
	files, err := os.ReadDir(p.storageDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plan storage directory %s: %w", p.storageDir, err)
	}

	var plansInfo []PlanInfo
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			planName := strings.TrimSuffix(file.Name(), ".json")

			plan, err := p.Get(planName) // Load the plan to get its details
			if err != nil {
				// Log the error and skip this plan, or return immediately?
				// For now, let's log and continue, so a single corrupted plan doesn't break the whole list.
				// Consider making this behavior configurable or returning partial results + an error list.
				fmt.Fprintf(os.Stderr, "warning: failed to load plan '%s' for listing: %v\n", planName, err)
				continue
			}

			totalTasks := len(plan.Steps)
			completedTasks := 0
			for _, step := range plan.Steps {
				if strings.ToUpper(step.status) == "DONE" {
					completedTasks++
				}
			}

			status := "TODO"
			if plan.IsCompleted() {
				status = "DONE"
			}

			plansInfo = append(plansInfo, PlanInfo{
				Name:           planName,
				Status:         status,
				TotalTasks:     totalTasks,
				CompletedTasks: completedTasks,
			})
		}
	}
	return plansInfo, nil
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

// Compact removes all completed plans from the storage directory.
// A plan is considered completed if all its steps are marked as "DONE".
func (p *Planner) Compact() error {
	files, err := os.ReadDir(p.storageDir)
	if err != nil {
		return fmt.Errorf("failed to read plan storage directory %s for compacting: %w", p.storageDir, err)
	}

	var compactedCount int // To keep track of how many plans are removed.
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			planName := strings.TrimSuffix(file.Name(), ".json")
			plan, err := p.Get(planName) // Use existing Get method to load plan details
			if err != nil {
				// Log error and continue, so a single corrupted/unreadable plan doesn't stop compaction.
				fmt.Fprintf(os.Stderr, "warning: failed to load plan '%s' during compact: %v\n", planName, err)
				continue
			}

			if plan.IsCompleted() { // Use existing IsCompleted method
				planPath := filepath.Join(p.storageDir, file.Name())
				if err := os.Remove(planPath); err != nil {
					// Log error and continue. Perhaps the file was removed by another process or permissions issue.
					fmt.Fprintf(os.Stderr, "warning: failed to remove completed plan file '%s': %v\n", planPath, err)
					continue
				}
				compactedCount++
				// Optional: Log successful removal for verbosity if desired.
				// fmt.Printf("Compacted (removed) completed plan: %s\n", planName)
			}
		}
	}

	// Optional: Log overall result for verbosity if desired.
	// fmt.Printf("Compaction complete. Removed %d completed plan(s).\n", compactedCount)
	return nil
}
