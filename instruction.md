It is time to refactor the planner/ module.

The original design goal was to have the following pattern:

1. plans.Create returns an memory-based plan instance,
2. methods on the plan instance change the state of the plan,
3. Only when plans.Save(plan) is called, the plan is persisted to the database.

First evaluate the current design of the planning module based on planner/planner.md and corresponding code files.

Then make a report of how what is implemented right now differs from the intended design.

Then make a detailed plan `planner-refactor` to change the code to match the design.
