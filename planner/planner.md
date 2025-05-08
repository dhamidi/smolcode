# Planner Module Documentation

The Planner module provides functionality for creating, managing, and persisting plans. Plans consist of a series of steps, each with a description, status, and acceptance criteria.

## Overview

The module is designed around a `Planner` type, which acts as the main interface for interacting with plans. Plans are stored as JSON files in a specified storage directory. Each plan has a unique ID (which is also its name and the base of its filename) and contains a list of steps.

## Core Concepts

### Planner

The `Planner` struct is the entry point for all plan management operations. It holds the path to the storage directory where plan files are kept.

- `New(storageDir string) (*Planner, error)`: Creates a new `Planner` instance. It ensures the specified `storageDir` exists, creating it if necessary.

### Plan

The `Plan` struct represents a collection of steps.

- `ID`: A unique identifier for the plan (e.g., "active"). This ID is used as the filename (e.g., "active.json").
- `Steps`: A slice of `*Step` pointers.
- `name`: An internal field representing the plan's name, typically derived from the filename.

#### Plan Methods

- `Create(name string) *Plan`: (Associated with `Planner`) Initializes a new `Plan` in memory with the given name. The plan is not persisted until `Save` is called.
- `Get(name string) (*Plan, error)`: (Associated with `Planner`) Retrieves a plan by its name from the storage directory.
- `Save(plan *Plan) error`: (Associated with `Planner`) Writes the given plan to a JSON file in the planner's storage directory.
- `Remove(planNames []string) map[string]error`: (Associated with `Planner`) Attempts to delete plans by their names from the storage directory. Returns a map of plan names to errors (nil on success).
- `List() ([]PlanInfo, error)`: (Associated with `Planner`) Returns summary information for all plans in the storage directory.
- `Compact() error`: (Associated with `Planner`) Removes all completed plans (where all steps are "DONE") from the storage directory.

- `Inspect() string`: (Method of `Plan`) Returns a string representation of the plan, formatted for display, showing each step's number, status, ID, description, and acceptance criteria.
- `NextStep() *Step`: (Method of `Plan`) Returns the first step in the plan that is not marked as "DONE". Returns `nil` if all steps are completed.
- `MarkAsCompleted(stepID string) error`: (Method of `Plan`) Finds a step by its ID and sets its status to "DONE".
- `MarkAsIncomplete(stepID string) error`: (Method of `Plan`) Finds a step by its ID and sets its status to "TODO".
- `AddStep(id, description string, acceptanceCriteria []string)`: (Method of `Plan`) Appends a new step to the plan. The new step is initialized with status "TODO".
- `RemoveSteps(stepIDs []string) int`: (Method of `Plan`) Removes steps from the plan based on a slice of step IDs. Returns the count of removed steps.
- `Reorder(newStepOrder []string)`: (Method of `Plan`) Rearranges the steps in the plan according to the `newStepOrder`. Steps in `newStepOrder` come first, followed by remaining steps in their original relative order.
- `IsCompleted() bool`: (Method of `Plan`) Checks if all steps in the plan are marked as "DONE".

### Step

The `Step` struct represents a single task within a plan.

- `id`: A short identifier for the step (e.g., "add-tests").
- `description`: A textual description of the step.
- `status`: The current status of the step, either "DONE" or "TODO".
- `acceptance`: A slice of strings representing the acceptance criteria for the step.

#### Step Methods

- `ID() string`: Returns the step's ID.
- `Status() string`: Returns the step's status (always uppercase).
- `Description() string`: Returns the step's description.
- `AcceptanceCriteria() []string`: Returns the step's acceptance criteria.

### PlanInfo

The `PlanInfo` struct holds summary information about a plan, used by the `Planner.List()` method.

- `Name`: The name of the plan.
- `Status`: Overall status of the plan ("DONE" or "TODO").
- `TotalTasks`: The total number of steps in the plan.
- `CompletedTasks`: The number of completed steps in the plan.

## Internal Storage

Plans are stored as individual JSON files within the `storageDir` provided when a `Planner` is instantiated.

- **Filename**: Each plan is stored in a file named `<plan_name>.json` (e.g., `main.json`, `feature-x.json`). The `plan_name` corresponds to the `ID` field of the `Plan` struct.
- **Format**: The JSON file stores a serialized version of the plan. To handle Go's private fields during JSON marshaling/unmarshaling, the module uses internal `serializablePlan` and `serializableStep` structs. These structs have exported fields (e.g., `Id` instead of `id`) that match the JSON structure.
  - `serializablePlan`: Contains `ID` and `Steps` (a slice of `*serializableStep`).
  - `serializableStep`: Contains `Id`, `Description`, `Status`, and `Acceptance`.
- **Serialization/Deserialization**:
  - When saving a plan (`Planner.Save`), the `Plan` and its `Step`s are converted to `serializablePlan` and `serializableStep`s before being marshaled to JSON.
  - When retrieving a plan (`Planner.Get`), the JSON data is first unmarshaled into `serializablePlan` and `serializableStep`s, and then converted back to `Plan` and `Step`s with their respective unexported fields populated.

### Example JSON Structure (`myplan.json`)

```json
{
  "ID": "myplan",
  "Steps": [
    {
      "Id": "step1",
      "Description": "This is the first step.",
      "Status": "TODO",
      "Acceptance": ["Criterion A for step 1", "Criterion B for step 1"]
    },
    {
      "Id": "step2",
      "Description": "This is the second step, already done.",
      "Status": "DONE",
      "Acceptance": []
    }
  ]
}
```

This structure allows for easy human readability and machine parsing of plan files. The use of intermediate serializable structs is a common Go pattern to control JSON representation independently of the primary domain model structs, especially when dealing with unexported fields or needing different field names in the JSON.
