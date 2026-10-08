# flow dry-run review: one task's text, read cold, before anything is filed.
# The parent reads this run's outcome from its terminal state name.

workflow(
    name = "dryrun-review",
    inputs = {"task": "dict", "trunk": "string"},
    start = "review",
)

state("review",
      agent("reviewer", "Follow the flow-review skill in dry-run mode.", skills = ["flow-review"],
            inputs = ["task", "trunk"]),
      clean = "clean", blockers = "blockers", wrong_ac = "wrong_ac")

state("clean", done())
state("blockers", done("{findings}"))
state("wrong_ac", done("{findings}"))
