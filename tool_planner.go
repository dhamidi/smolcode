package smolcode

import (
	"fmt"
	"log"
	"strings"

	"github.com/dhamidi/smolcode/planner"
	"google.golang.org/genai"
)

const planStoragePath = ".smolcode/plans.db"

// Define the Step structure for parameter definition
var plannerStepSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"id": {
			Type:        genai.TypeString,
			Description: "A short, unique identifier for the step (e.g., 'add-tests').",
		},
		"description": {
			Type:        genai.TypeString,
			Description: "A detailed description of the step's task.",
		},
		"acceptance_criteria": {
			Type: genai.TypeArray,
			Items: &genai.Schema{
				Type: genai.TypeString,
			},
			Description: "A list of criteria that must be met for the step to be considered DONE.",
		},
		// Status is implicitly TODO when adding steps.
	},
	Required: []string{"id", "description"}, // Acceptance criteria are optional
}

var PlannerTool = &ToolDefinition{
	Tool: &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "manage_plan",
				Description: strings.TrimSpace(`
Manages development plans. Use this tool to create, inspect, modify, and query the status of plans and their steps.
Plans are stored in '.smolcode/plans/'. Always specify the plan name.
`),
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"plan_name": {
							Type:        genai.TypeString,
							Description: "The name of the plan to manage (e.g., 'main', 'feature-x'). This corresponds to the filename without .json.",
						},
						"action": {
							Type: genai.TypeString,
							Enum: []string{
								"inspect",       // Get the Markdown representation of the plan.
								"get_next_step", // Get details of the next incomplete step.
								"set_status",    // Mark a specific step as DONE or TODO.
								"add_steps",     // Add one or more new steps to the end of the plan, creating it if necessary
								"is_completed",  // Check if all steps in the plan are DONE.
								"list_plans",    // List all available plan names.
								"remove_steps",  // Remove specified steps from a plan.
								"compact_plans", // Remove all completed plan files. The plan_name argument will be ignored for this action.
								"reorder_steps", // Reorder steps within a plan according to a given list of step IDs.
							},
							Description: "The operation to perform on the plan.",
						},
						// Parameters specific to certain actions
						"step_id": {
							Type:        genai.TypeString,
							Description: "The ID of the step to target (required for 'set_status').",
						},
						"status": {
							Type:        genai.TypeString,
							Enum:        []string{"DONE", "TODO"},
							Description: "The status to set for a step (required for 'set_status').",
						},
						"steps_to_add": {
							Type:        genai.TypeArray,
							Items:       plannerStepSchema,
							Description: "A list of step objects to add to the plan (required for 'add_steps'), creating it if necessary.",
						},
						"step_ids_to_remove": {
							Type: genai.TypeArray,
							Items: &genai.Schema{
								Type: genai.TypeString,
							},
							Description: "A list of step IDs to remove from the plan (required for 'remove_steps').",
						},
						"new_step_order": {
							Type: genai.TypeArray,
							Items: &genai.Schema{
								Type: genai.TypeString,
							},
							Description: "A list of step IDs representing the desired new order (required for 'reorder_steps'). Steps not in this list are appended at the end.",
						},
					},
					Required: []string{"plan_name", "action"},
				},
			},
		},
	},
	Function: managePlan, // To be implemented
}

// Function implementation
func managePlan(args map[string]any) (map[string]any, error) {
	// 1. Extract arguments (plan_name, action, etc.) with type assertions and validation.
	plannerName, ok := args["plan_name"].(string)
	if !ok || plannerName == "" {
		return nil, fmt.Errorf("manage_plan: missing or invalid plan_name")
	}
	action, ok := args["action"].(string)
	if !ok || action == "" {
		return nil, fmt.Errorf("manage_plan: missing or invalid action")
	}

	// 2. Initialize planner: plans, err := planner.New(".smolcode/plans/") (handle error)
	plans, err := planner.New(planStoragePath)
	if err != nil {
		return nil, fmt.Errorf("manage_plan: failed to initialize planner: %w", err)
	}

	// 3. Use a switch on the 'action'.
	switch action {
	case "inspect":
		plan, err := plans.Get(plannerName)
		if err != nil {
			return nil, fmt.Errorf("manage_plan: failed to get plan '%s': %w", plannerName, err)
		}
		return map[string]any{"markdown": plan.Inspect()}, nil

	case "get_next_step":
		plan, err := plans.Get(plannerName)
		if err != nil {
			return nil, fmt.Errorf("manage_plan: failed to get plan '%s': %w", plannerName, err)
		}
		next := plan.NextStep()
		if next == nil {
			return map[string]any{"result": "Plan is complete."}, nil
		} else {
			return map[string]any{
				"next_step": map[string]any{
					"id":                  next.ID(),
					"status":              next.Status(),
					"description":         next.Description(),
					"acceptance_criteria": next.AcceptanceCriteria(),
				},
			}, nil
		}

	case "set_status":
		stepID, ok := args["step_id"].(string)
		if !ok || stepID == "" {
			return nil, fmt.Errorf("manage_plan: 'set_status' requires 'step_id'")
		}
		status, ok := args["status"].(string)
		if !ok || (status != "DONE" && status != "TODO") {
			return nil, fmt.Errorf("manage_plan: 'set_status' requires 'status' (DONE or TODO)")
		}

		retrievedPlan, err := plans.Get(plannerName)
		if err != nil {
			return nil, fmt.Errorf("manage_plan: failed to get plan '%s' for set_status: %w", plannerName, err)
		}

		if status == "DONE" {
			err = retrievedPlan.MarkAsCompleted(stepID)
		} else {
			err = retrievedPlan.MarkAsIncomplete(stepID)
		}

		if err != nil {
			return nil, fmt.Errorf("manage_plan: failed to set status for step '%s' in plan '%s': %w", stepID, plannerName, err)
		}

		// Persist the change to the plan (including the updated step status)
		if err = plans.Save(retrievedPlan); err != nil {
			return nil, fmt.Errorf("manage_plan: failed to save plan '%s' after setting status: %w", plannerName, err)
		}

		return map[string]any{"result": fmt.Sprintf("Step '%s' in plan '%s' set to '%s'.", stepID, plannerName, status)}, nil

	case "add_steps":
		stepsToAddArg, ok := args["steps_to_add"].([]any)
		if !ok {
			return nil, fmt.Errorf("manage_plan: 'add_steps' requires 'steps_to_add' array")
		}

		plan, err := plans.Get(plannerName)
		if err != nil {
			// Check if the error is a "plan not found" type error
			// planner.Get now returns an error like "plan with name '...' not found"
			// planner.Create also returns an error if it fails
			if strings.Contains(strings.ToLower(err.Error()), "not found") { // Make check case-insensitive for robustness
				log.Printf("manage_plan: Plan '%s' not found, creating it before adding steps.", plannerName)
				plan, err = plans.Create(plannerName) // Assign to plan and new err
				if err != nil {
					return nil, fmt.Errorf("manage_plan: failed to create new plan '%s' for adding steps: %w", plannerName, err)
				}
			} else { // Any other error from Get
				return nil, fmt.Errorf("manage_plan: failed to get plan '%s': %w", plannerName, err)
			}
		}
		// If err was nil from Get, plan is already populated and ready

		addedCount := 0
		for i, stepArg := range stepsToAddArg {
			stepMap, ok := stepArg.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("manage_plan: invalid item type in 'steps_to_add' at index %d, expected object", i)
			}

			id, ok := stepMap["id"].(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("manage_plan: missing 'id' in step at index %d", i)
			}
			description, ok := stepMap["description"].(string)
			if !ok || description == "" {
				return nil, fmt.Errorf("manage_plan: missing 'description' in step '%s' at index %d", id, i)
			}

			var criteria []string
			if criteriaArg, present := stepMap["acceptance_criteria"].([]any); present {
				for j, critArg := range criteriaArg {
					critStr, ok := critArg.(string)
					if !ok {
						return nil, fmt.Errorf("manage_plan: invalid acceptance criterion type in step '%s' at index %d, criterion %d", id, i, j)
					}
					criteria = append(criteria, critStr)
				}
			}

			plan.AddStep(id, description, criteria)
			addedCount++
		}

		if err := plans.Save(plan); err != nil {
			return nil, fmt.Errorf("manage_plan: failed to save updated plan '%s': %w", plannerName, err)
		}
		return map[string]any{"result": fmt.Sprintf("Added %d steps to plan '%s'.", addedCount, plannerName)}, nil

	case "is_completed":
		plan, err := plans.Get(plannerName)
		if err != nil {
			return nil, fmt.Errorf("manage_plan: failed to get plan '%s': %w", plannerName, err)
		}
		isCompleted := plan.IsCompleted()
		return map[string]any{"is_completed": isCompleted}, nil

	case "list_plans":
		plansInfo, err := plans.List() // planner.List() now returns []planner.PlanInfo
		if err != nil {
			return nil, fmt.Errorf("manage_plan: failed to list plans: %w", err)
		}
		// The planner.PlanInfo struct has json tags, so it will be marshalled correctly.
		return map[string]any{"plans": plansInfo}, nil

	case "remove_steps":
		stepIDsToRemoveArg, ok := args["step_ids_to_remove"].([]any)
		if !ok {
			// Enforce it must be present, can be empty, as per schema for array types.
			return nil, fmt.Errorf("manage_plan: 'remove_steps' requires 'step_ids_to_remove' array argument")
		}

		var stepIDs []string
		for i, idArg := range stepIDsToRemoveArg {
			idStr, ok := idArg.(string)
			if !ok || idStr == "" {
				return nil, fmt.Errorf("manage_plan: invalid or empty step ID in 'step_ids_to_remove' at index %d", i)
			}
			stepIDs = append(stepIDs, idStr)
		}

		plan, err := plans.Get(plannerName)
		if err != nil {
			return nil, fmt.Errorf("manage_plan: failed to get plan '%s' for removing steps: %w", plannerName, err)
		}

		removedCount := plan.RemoveSteps(stepIDs) // This is the call to the method added in planner.go

		if err := plans.Save(plan); err != nil {
			return nil, fmt.Errorf("manage_plan: failed to save plan '%s' after removing steps: %w", plannerName, err)
		}
		return map[string]any{
			"result":        fmt.Sprintf("Removed %d step(s) from plan '%s'.", removedCount, plannerName),
			"removed_count": removedCount,
			"plan_name":     plannerName,
		}, nil

	case "compact_plans":
		// plannerName is ignored for this action, as compaction is global.
		// The planner instance 'plans' is already initialized.
		err := plans.Compact() // This is the call to the method added in planner.go
		if err != nil {
			// The Compact method in planner.go logs warnings for individual file errors
			// but returns an error if the directory read fails.
			return nil, fmt.Errorf("manage_plan: 'compact_plans' action encountered an error: %w", err)
		}
		return map[string]any{
			"result": "Plan compaction process completed. Check server logs for details on individual plan loading/removal warnings.",
		}, nil

	case "reorder_steps":
		newStepOrderArg, ok := args["new_step_order"].([]any)
		if !ok {
			// new_step_order is effectively required by this action.
			return nil, fmt.Errorf("manage_plan: 'reorder_steps' requires 'new_step_order' array argument")
		}

		var newStepOrder []string
		for i, idArg := range newStepOrderArg {
			idStr, ok := idArg.(string)
			if !ok { // Step IDs can be anything, but must be strings. Empty string ID might be an issue for some systems but planner.go allows it.
				return nil, fmt.Errorf("manage_plan: invalid type for step ID in 'new_step_order' at index %d, expected string", i)
			}
			// We allow empty strings as step IDs if the planner package supports them.
			// The Reorder method in planner.go itself will ignore IDs not found.
			newStepOrder = append(newStepOrder, idStr)
		}

		plan, err := plans.Get(plannerName)
		if err != nil {
			return nil, fmt.Errorf("manage_plan: failed to get plan '%s' for reordering steps: %w", plannerName, err)
		}

		plan.Reorder(newStepOrder) // Call the in-memory reorder method

		if err := plans.Save(plan); err != nil { // Persist the changes
			return nil, fmt.Errorf("manage_plan: failed to save plan '%s' after reordering steps: %w", plannerName, err)
		}
		return map[string]any{
			"result":    fmt.Sprintf("Steps in plan '%s' reordered successfully.", plannerName),
			"plan_name": plannerName,
		}, nil

	default:
		return nil, fmt.Errorf("manage_plan: unknown action '%s'", action)
	}
}
