Our next task is to create a planner module in a directory called planner/

The planner can be used like this:

```go
// create a planner storing plans in .smolcode/plans
plans := planner.New(".smolcode/plans/")

// create a new plan in memory (not saved yet)
newPlan := plans.Create("my-new-plan")

// get the active plan from .smolcode/plans/active.json
activePlan, err := plans.Get("active")

// converts the plan to Markdown
// each step is a headline
// the description is a paragraph
// acceptance criteria become a numbered list
// the step status is indicated in the headline "DONE" for completed steps, "TODO" for incomplete/not-started steps
asMarkdown := activePlan.Inspect()

// get the next step that is not completed yet
step : = activePlan.NextStep()

// returns the ID of the step as a short identifier, e.g. `add-tests`
step.ID() 

// returns the status of the step as text: either "DONE" or "TODO"
step.Status()

// returns a text description of the step
step.Description()

// returns a list of acceptance criteria
checks := step.AcceptanceCriteria()

// marks a step as DONE
activePlan.MarkAsCompleted(step.ID())

// marks a step as TODO
activePlan.MarkAsIncomplete(step.ID())

// returns true if all steps of the plan have been completed
activePlan.IsCompleted()

// writes the plan back to disk as a json file
err := planner.Save(activePlan) 
```

# Coding guidelines

All struct fields in the planner package should be private.

Communication with the plan should only happen through the public interface and method calls.
