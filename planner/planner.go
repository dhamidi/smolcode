package planner

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Planner manages plans using a SQLite database.
type Planner struct {
	db *sql.DB
}

// Plan represents a collection of steps.
type Plan struct {
	ID    string  `json:"id"` // Unique identifier for the plan, e.g., "active"
	Steps []*Step `json:"steps"`
}

// PlanInfo holds summary information about a plan.
// This is used by the List method.
type PlanInfo struct {
	Name           string `json:"name"`
	Status         string `json:"status"` // "DONE" or "TODO"
	TotalTasks     int    `json:"total_tasks"`
	CompletedTasks int    `json:"completed_tasks"`
}

// Step represents a single task in a plan.
type Step struct {
	id          string   `json:"id"` // Short identifier, e.g., "add-tests"
	description string   `json:"description"`
	status      string   `json:"status"` // "DONE" or "TODO"
	acceptance  []string `json:"acceptance"`
	stepOrder   int      // Internal field to keep track of order from DB
}

// New creates a new Planner instance connected to a SQLite database.
// It ensures the database and necessary tables are initialized.
// databasePath specifies the path to the SQLite database file.
func New(databasePath string) (*Planner, error) {
	// Ensure the directory for the database file exists.
	dbDir := filepath.Dir(databasePath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for database %s: %w", dbDir, err)
	}

	db, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database at %s: %w", databasePath, err)
	}

	// Enable foreign key constraints
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		db.Close() // Close the DB if PRAGMA fails
		return nil, fmt.Errorf("failed to enable foreign key constraints: %w", err)
	}

	// Read schema.sql file
	// Assuming schema.sql is in the same directory as this planner.go file.
	// For a real application, this path might need to be configurable or embedded.
	schemaPath := filepath.Join(filepath.Dir(databasePath), "schema.sql") // Adjusted to be relative to db path for now
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		// If schema.sql is not found next to db, try to find it next to the executable or in `planner/schema.sql`
		exePath, _ := os.Executable()
		schemaPath = filepath.Join(filepath.Dir(exePath), "planner", "schema.sql")
		if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
			schemaPath = "planner/schema.sql" // Fallback for tests or specific structures
		}
	}

	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to read schema file %s: %w", schemaPath, err)
	}

	// Execute schema
	_, err = db.Exec(string(schemaSQL))
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	return &Planner{
		db: db,
	}, nil
}

// Close closes the database connection.
// It is the caller's responsibility to close the planner when done.
func (p *Planner) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

// Create inserts a new plan into the 'plans' table and returns an in-memory Plan object.
// The ID of the plan is set to its name.
func (p *Planner) Create(name string) (*Plan, error) {
	if name == "" {
		return nil, fmt.Errorf("plan name cannot be empty")
	}

	_, err := p.db.Exec("INSERT INTO plans (id) VALUES (?)", name)
	if err != nil {
		// Check if the error is due to a unique constraint violation (plan already exists)
		// This error message might be SQLite specific.
		// For more robust error handling, consider using specific error types if the driver provides them.
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, fmt.Errorf("plan with name '%s' already exists", name)
		}
		return nil, fmt.Errorf("failed to insert plan '%s' into database: %w", name, err)
	}

	return &Plan{
		ID:    name,
		Steps: []*Step{},
	}, nil
}

// Get retrieves a plan and its steps from the database.
func (p *Planner) Get(name string) (*Plan, error) {
	var planID string
	err := p.db.QueryRow("SELECT id FROM plans WHERE id = ?", name).Scan(&planID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("plan with name '%s' not found", name)
		}
		return nil, fmt.Errorf("failed to query plan '%s': %w", name, err)
	}

	plan := &Plan{
		ID:    planID,
		Steps: []*Step{},
	}

	rows, err := p.db.Query("SELECT id, description, status, step_order FROM steps WHERE plan_id = ? ORDER BY step_order ASC", planID)
	if err != nil {
		return nil, fmt.Errorf("failed to query steps for plan '%s': %w", name, err)
	}
	defer rows.Close()

	// Use a map to temporarily store steps by ID for efficient lookup when adding acceptance criteria
	stepsByID := make(map[string]*Step)

	for rows.Next() {
		step := &Step{}
		err := rows.Scan(&step.id, &step.description, &step.status, &step.stepOrder)
		if err != nil {
			return nil, fmt.Errorf("failed to scan step for plan '%s': %w", name, err)
		}
		step.acceptance = []string{} // Initialize acceptance criteria slice
		plan.Steps = append(plan.Steps, step)
		stepsByID[step.id] = step // Store step by ID for later lookup
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating steps for plan '%s': %w", name, err)
	}

	// Now, fetch acceptance criteria for each step
	// Iterate over the plan.Steps to maintain the order from the database query
	for _, step := range plan.Steps {
		acRows, err := p.db.Query("SELECT criterion FROM step_acceptance_criteria WHERE step_id = ? AND plan_id = ? ORDER BY criterion_order ASC", step.id, planID)
		if err != nil {
			return nil, fmt.Errorf("failed to query acceptance criteria for step '%s' in plan '%s': %w", step.id, name, err)
		}
		// It's important to close acRows in each iteration to prevent resource leaks.
		// Using defer here might be tricky due to the loop, so manual close is better.

		for acRows.Next() {
			var acDescription string
			err := acRows.Scan(&acDescription)
			if err != nil {
				acRows.Close() // Ensure closure on error
				return nil, fmt.Errorf("failed to scan acceptance criterion for step '%s' in plan '%s': %w", step.id, name, err)
			}
			step.acceptance = append(step.acceptance, acDescription)
		}
		if err = acRows.Err(); err != nil {
			acRows.Close() // Ensure closure on error
			return nil, fmt.Errorf("error iterating acceptance criteria for step '%s' in plan '%s': %w", step.id, name, err)
		}
		acRows.Close() // Close after successful iteration
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

// MarkAsCompleted finds a step by its ID within a specific plan and sets its status to "DONE" in the database and in-memory.
// It also updates the in-memory plan object if provided.
func (p *Planner) MarkAsCompleted(planID string, stepID string, currentPlan *Plan) error {
	result, err := p.db.Exec("UPDATE steps SET status = ? WHERE plan_id = ? AND id = ?", "DONE", planID, stepID)
	if err != nil {
		return fmt.Errorf("failed to update step '%s' in plan '%s' to DONE: %w", stepID, planID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		// This error is less critical for the operation itself but good for diagnostics.
		return fmt.Errorf("failed to get rows affected after updating step '%s' in plan '%s': %w", stepID, planID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("step with ID '%s' not found in plan '%s' in the database", stepID, planID)
	}

	// Update in-memory plan if provided
	if currentPlan != nil && currentPlan.ID == planID {
		found := false
		for _, step := range currentPlan.Steps {
			if step.id == stepID {
				step.status = "DONE"
				found = true
				break
			}
		}
		if !found {
			// This indicates a discrepancy between DB and in-memory, which might be an issue.
			// For now, we'll just return an error or warning.
			return fmt.Errorf("step '%s' updated in DB but not found in provided in-memory plan '%s'", stepID, planID)
		}
	}
	return nil
}

// MarkAsIncomplete finds a step by its ID  within a specific plan and sets its status to "TODO" in the database and in-memory.
// It also updates the in-memory plan object if provided.
func (p *Planner) MarkAsIncomplete(planID string, stepID string, currentPlan *Plan) error {
	result, err := p.db.Exec("UPDATE steps SET status = ? WHERE plan_id = ? AND id = ?", "TODO", planID, stepID)
	if err != nil {
		return fmt.Errorf("failed to update step '%s' in plan '%s' to TODO: %w", stepID, planID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected after updating step '%s' in plan '%s': %w", stepID, planID, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("step with ID '%s' not found in plan '%s' in the database", stepID, planID)
	}

	// Update in-memory plan if provided
	if currentPlan != nil && currentPlan.ID == planID {
		found := false
		for _, step := range currentPlan.Steps {
			if step.id == stepID {
				step.status = "TODO"
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("step '%s' updated in DB but not found in provided in-memory plan '%s'", stepID, planID)
		}
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

// RemoveSteps removes steps from the plan based on the provided slice of step IDs.
// It returns the number of steps actually removed.
// It is not an error if a provided step ID is not found in the plan.
func (pl *Plan) RemoveSteps(stepIDs []string) int {
	if len(stepIDs) == 0 {
		return 0 // Nothing to remove
	}
	if len(pl.Steps) == 0 {
		return 0 // No steps in the plan to remove from
	}

	// Create a set of IDs to remove for efficient lookup
	idsToRemove := make(map[string]struct{})
	for _, id := range stepIDs {
		idsToRemove[id] = struct{}{}
	}

	var newSteps []*Step
	removedCount := 0
	for _, step := range pl.Steps {
		if _, found := idsToRemove[step.id]; found {
			removedCount++
		} else {
			newSteps = append(newSteps, step)
		}
	}

	pl.Steps = newSteps
	return removedCount
}

// Reorder rearranges the steps in the plan.
// Steps whose IDs are in newStepOrder are placed first, in the specified order.
// Any remaining steps from the original plan are appended afterwards,
// maintaining their original relative order.
// If a step ID in newStepOrder does not exist in the plan, it is ignored.
// Duplicate step IDs in newStepOrder are also effectively ignored after the first placement.
func (pl *Plan) Reorder(newStepOrder []string) {
	if len(pl.Steps) == 0 {
		return // Nothing to reorder
	}

	originalStepsMap := make(map[string]*Step, len(pl.Steps))
	for _, step := range pl.Steps {
		originalStepsMap[step.id] = step
	}

	var reorderedSteps []*Step
	// Keep track of steps that have been explicitly placed by newStepOrder
	// to correctly append remaining steps and handle potential duplicates in newStepOrder.
	placedStepIDs := make(map[string]struct{})

	// First, place steps according to newStepOrder
	for _, stepID := range newStepOrder {
		step, exists := originalStepsMap[stepID]
		if !exists {
			continue // Step ID from newStepOrder not found in plan, ignore.
		}
		if _, alreadyPlaced := placedStepIDs[stepID]; alreadyPlaced {
			continue // Step ID was already placed (e.g., duplicate in newStepOrder), ignore.
		}
		reorderedSteps = append(reorderedSteps, step)
		placedStepIDs[stepID] = struct{}{}
	}

	// Then, append any remaining steps from the original order
	// that were not part of newStepOrder (or were duplicates and thus not re-added).
	for _, originalStep := range pl.Steps {
		if _, wasPlaced := placedStepIDs[originalStep.id]; !wasPlaced {
			reorderedSteps = append(reorderedSteps, originalStep)
			// Mark as placed here too, though less critical as we iterate originalSteps once.
			placedStepIDs[originalStep.id] = struct{}{}
		}
	}

	pl.Steps = reorderedSteps
}

// IsCompleted checks if all steps in the plan are marked as "DONE".
func (pl *Plan) IsCompleted() bool {
	return pl.NextStep() == nil // If NextStep is nil, all steps are DONE
}

// List retrieves summary information for all plans from the database.
func (p *Planner) List() ([]PlanInfo, error) {
	rows, err := p.db.Query(`
        SELECT 
            p.id, 
            COUNT(s.id),
            SUM(CASE WHEN s.status = 'DONE' THEN 1 ELSE 0 END)
        FROM plans p
        LEFT JOIN steps s ON p.id = s.plan_id
        GROUP BY p.id
    `)
	if err != nil {
		return nil, fmt.Errorf("failed to query plan summaries: %w", err)
	}
	defer rows.Close()

	var plansInfo []PlanInfo
	for rows.Next() {
		var info PlanInfo
		var totalTasks sql.NullInt64     // Use NullInt64 for COUNT which can be 0 -> NULL
		var completedTasks sql.NullInt64 // Use NullInt64 for SUM which can be NULL if no rows

		if err := rows.Scan(&info.Name, &totalTasks, &completedTasks); err != nil {
			return nil, fmt.Errorf("failed to scan plan summary: %w", err)
		}

		info.TotalTasks = int(totalTasks.Int64)         // Assign, defaults to 0 if NULL
		info.CompletedTasks = int(completedTasks.Int64) // Assign, defaults to 0 if NULL

		if info.TotalTasks > 0 && info.CompletedTasks == info.TotalTasks {
			info.Status = "DONE"
		} else {
			info.Status = "TODO"
		}
		plansInfo = append(plansInfo, info)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating plan summaries: %w", err)
	}

	return plansInfo, nil
}

// Save persists changes to a plan and its steps in the database using a transaction.
func (p *Planner) Save(plan *Plan) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback if not committed

	// In a real application, you might update a 'last_modified' timestamp in the 'plans' table here.
	// For now, we assume the plan record itself (just an ID) doesn\'t change once created.

	// --- Synchronize steps --- //

	// Get existing step IDs from the DB for this plan
	rows, err := tx.Query("SELECT id FROM steps WHERE plan_id = ?", plan.ID)
	if err != nil {
		return fmt.Errorf("failed to query existing steps for plan '%s': %w", plan.ID, err)
	}
	dbStepIDs := make(map[string]bool)
	for rows.Next() {
		var stepID string
		if err := rows.Scan(&stepID); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan existing step ID: %w", err)
		}
		dbStepIDs[stepID] = true
	}
	rows.Close()                      // Important to close rows before further operations on tx
	if err = rows.Err(); err != nil { // Check for errors during iteration
		return fmt.Errorf("error iterating existing step IDs: %w", err)
	}

	// Identify steps to delete, update, or insert
	planStepIDs := make(map[string]bool)
	for _, step := range plan.Steps {
		planStepIDs[step.id] = true
	}

	// Delete steps not in the current plan.Steps
	for dbStepID := range dbStepIDs {
		if !planStepIDs[dbStepID] {
			// Delete acceptance criteria for the step first (due to foreign key)
			_, err = tx.Exec("DELETE FROM step_acceptance_criteria WHERE plan_id = ? AND step_id = ?", plan.ID, dbStepID)
			if err != nil {
				return fmt.Errorf("failed to delete old acceptance criteria for step '%s' in plan '%s': %w", dbStepID, plan.ID, err)
			}
			// Then delete the step itself
			_, err = tx.Exec("DELETE FROM steps WHERE plan_id = ? AND id = ?", plan.ID, dbStepID)
			if err != nil {
				return fmt.Errorf("failed to delete step '%s' from plan '%s': %w", dbStepID, plan.ID, err)
			}
		}
	}

	// Update existing steps or insert new ones
	for i, step := range plan.Steps {
		step.stepOrder = i      // Ensure stepOrder is current
		if dbStepIDs[step.id] { // If step exists in DB, update it
			_, err = tx.Exec("UPDATE steps SET description = ?, status = ?, step_order = ? WHERE plan_id = ? AND id = ?",
				step.description, step.status, step.stepOrder, plan.ID, step.id)
			if err != nil {
				return fmt.Errorf("failed to update step '%s' in plan '%s': %w", step.id, plan.ID, err)
			}
		} else { // Otherwise, insert it
			_, err = tx.Exec("INSERT INTO steps (id, plan_id, description, status, step_order) VALUES (?, ?, ?, ?, ?)",
				step.id, plan.ID, step.description, step.status, step.stepOrder)
			if err != nil {
				return fmt.Errorf("failed to insert step '%s' into plan '%s': %w", step.id, plan.ID, err)
			}
		}

		// --- Synchronize acceptance criteria for this step --- //
		// Delete all existing criteria for the current step first
		_, err = tx.Exec("DELETE FROM step_acceptance_criteria WHERE plan_id = ? AND step_id = ?", plan.ID, step.id)
		if err != nil {
			return fmt.Errorf("failed to delete old acceptance criteria for step '%s' in plan '%s': %w", step.id, plan.ID, err)
		}

		// Insert current acceptance criteria for the step
		for j, acText := range step.acceptance {
			_, err = tx.Exec("INSERT INTO step_acceptance_criteria (plan_id, step_id, criterion_order, criterion) VALUES (?, ?, ?, ?)",
				plan.ID, step.id, j, acText)
			if err != nil {
				return fmt.Errorf("failed to insert acceptance criterion for step '%s' in plan '%s': %w", step.id, plan.ID, err)
			}
		}
	}

	return tx.Commit()
}

// Remove deletes plans from the database by their names (IDs).
// It relies on "ON DELETE CASCADE" foreign key constraints to remove associated steps and criteria.
// It returns a map where keys are plan names and values are errors encountered during deletion (nil on success).
func (p *Planner) Remove(planNames []string) map[string]error {
	results := make(map[string]error)
	tx, err := p.db.Begin() // Start a transaction for potentially multiple deletes
	if err != nil {
		// If we can't even begin a transaction, report a general error.
		// We can't assign it to a specific plan name.
		// Alternatively, return a single error here.
		results["_"] = fmt.Errorf("failed to begin transaction for remove: %w", err)
		return results
	}
	defer tx.Rollback() // Ensure rollback on error

	stmt, err := tx.Prepare("DELETE FROM plans WHERE id = ?")
	if err != nil {
		results["_"] = fmt.Errorf("failed to prepare delete statement: %w", err)
		return results
	}
	defer stmt.Close()

	for _, name := range planNames {
		result, err := stmt.Exec(name)
		if err != nil {
			results[name] = fmt.Errorf("failed to execute delete for plan '%s': %w", name, err)
			continue // Continue trying to delete others
		}
		rowsAffected, _ := result.RowsAffected() // Check if the plan actually existed
		if rowsAffected == 0 {
			// Optionally report this as an error or warning
			results[name] = fmt.Errorf("plan '%s' not found for deletion", name)
		} else {
			results[name] = nil // Mark as success
		}
	}

	// Check if any specific errors occurred
	hasErrors := false
	for _, err := range results {
		if err != nil {
			hasErrors = true
			break
		}
	}

	if !hasErrors {
		if err := tx.Commit(); err != nil {
			results["_"] = fmt.Errorf("failed to commit transaction for remove: %w", err)
			// If commit fails, the actual outcome is uncertain. Mark all non-errored as failed?
			for name, resErr := range results {
				if resErr == nil {
					results[name] = fmt.Errorf("transaction commit failed after successful delete prep: %w", err)
				}
			}
		}
	} else {
		// Rollback happens automatically via defer, just return the results map with errors.
	}

	return results
}

// Compact removes all completed plans from the database.
// A plan is completed if it has no steps or all its steps are marked as 'DONE'.
func (p *Planner) Compact() error {
	query := `
        SELECT p.id
        FROM plans p
        LEFT JOIN steps s ON p.id = s.plan_id
        GROUP BY p.id
        HAVING COUNT(s.id) = 0 OR SUM(CASE WHEN s.status = 'DONE' THEN 1 ELSE 0 END) = COUNT(s.id);
    `
	rows, err := p.db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query completed plans for compaction: %w", err)
	}
	defer rows.Close()

	var completedPlanIDs []string
	for rows.Next() {
		var planID string
		if err := rows.Scan(&planID); err != nil {
			return fmt.Errorf("failed to scan completed plan ID: %w", err)
		}
		completedPlanIDs = append(completedPlanIDs, planID)
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("error iterating completed plan IDs: %w", err)
	}
	rows.Close() // Close rows before starting transaction

	if len(completedPlanIDs) == 0 {
		return nil // Nothing to compact
	}

	// Use the existing Remove method which handles transactions and cascading deletes
	// The Remove method returns a map of errors, but Compact just returns a single error.
	// We'll check the map for any errors.
	removeResults := p.Remove(completedPlanIDs)

	var firstError error
	var errorCount int
	for planID, err := range removeResults {
		if err != nil {
			errorCount++
			if firstError == nil {
				// Capture the first error encountered
				if planID == "_" { // Check for transaction level error from Remove
					firstError = err
				} else {
					firstError = fmt.Errorf("failed to remove plan '%s': %w", planID, err)
				}
			}
			// Optionally log subsequent errors if desired
			// fmt.Fprintf(os.Stderr, "warning: error during compact removal of plan '%s': %v\n", planID, err)
		}
	}

	if firstError != nil {
		return fmt.Errorf("encountered %d error(s) during compaction, first error: %w", errorCount, firstError)
	}

	// Optional: Log success
	// fmt.Printf("Compaction complete. Removed %d completed plan(s).\n", len(completedPlanIDs))
	return nil
}
