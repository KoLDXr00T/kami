# flow `start` + `followup`: idea → grounded tickets → one `ticket` run per task.

load("lib/multica.star", "preflight")

workflow(
    name = "goal",
    inputs = {"idea": "string", "env": "string?", "lane": "string?"},
    start = "preflight",
    afk = afk("attended"),
    entries = {"followup": entry("followup_plan", require = ["goal", "feedback"])},
)

def dryrun_verdict(v):
    if all([c["outcome"] == "clean" for c in v["children"]]):
        return "dispatch_gate"
    if v["visits"]["dryrun"] == 1:
        return "refine"  # one revision round; the dry run never runs twice
    return "decide"

def dryrun_input(task, v):
    return {"task": task, "trunk": v["trunk"]}

def ticket_input(task, v):
    return {"task": task, "goal": v["goal"], "env": v["env"], "trunk": v["trunk"], "lane": v.get("lane")}

preflight(then = "refine")

# ── Plan ────────────────────────────────────────────────────────────────

state("refine",
      agent("refiner", "Follow the flow-refine skill.", skills = ["flow-refine"],
            inputs = ["idea", "env", "trunk", "repos", "answers", "prior_draft", "objections"]),
      tasks = "ground", questions = "answers")

state("answers", ask("Refine needs answers before it can draft tickets", body_file = "{questions_md}"),
      answered = "refine")

state("ground",
      agent("grounder", "Follow the flow-ground skill.", skills = ["flow-ground"],
            inputs = ["idea", "answers", "tasks"], max_visits = 3),
      grounded = "dryrun", revise = "refine", needs_decision = "decide")

state("dryrun", spawn("dryrun-review", for_each = "tasks", input = dryrun_input),
      all_done = "dryrun_verdict", any_failed = "decide")

state("dryrun_verdict", choose(dryrun_verdict))

# ── Dispatch ────────────────────────────────────────────────────────────

state("dispatch_gate", gate("File these tickets?", body_file = "{tickets_md}"),
      file = "dispatch", stop = "stopped")

state("dispatch", spawn("ticket", for_each = "tasks", input = ticket_input),
      all_done = "shipped", any_failed = "decide")

# ── Followup entry ──────────────────────────────────────────────────────

state("followup_plan", script("scripts/goal-tickets {goal}"),
      found = "refine", none = "decide")

# ── Ends ────────────────────────────────────────────────────────────────

state("decide", choice("flow needs a decision", body_file = "{decision_md}"),
      revise = "refine", stop = "stopped")

state("shipped", done("merged, green, live: go test"))
state("stopped", cancel())
