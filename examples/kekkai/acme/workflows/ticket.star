# flow per ticket: file → builder → review loop → your merge → rollout.

load("lib/multica.star", "enqueue")

workflow(
    name = "ticket",
    inputs = {"task": "dict", "goal": "string", "env": "string", "trunk": "string", "lane": "string?"},
    start = "file",
)

# The review reads a head somebody pinned: these five come from one MR resolution.
REVIEW_INPUTS = ["ticket_id", "repo", "mr_iid", "head_sha", "trunk"]

def reviewer(v):
    return "reviewer-codex" if v.get("lane") == "codex" else "reviewer"

# ── File and start the builder ──────────────────────────────────────────

state("file", script("scripts/file-ticket"),
      filed = "await_run", adopted = "await_run")

enqueue(then = "poll_mr", on_dead = "lane")

# The answer lands in vars["lane"], which `reviewer` and file-ticket read.
state("lane", choice("ccpush failed. Move this ticket to another lane?"),
      codex = "file", stop = "failed")

# ── Build and review loop ───────────────────────────────────────────────

state("poll_mr", wait(probe = "scripts/mr-status {ticket_id}", every = "10m", timeout = "72h"),
      unreviewed = "review", merged_outside = "failed", timeout = "stalled")

state("review",
      agent(reviewer, "Follow the flow-review skill.", skills = ["flow-review"], inputs = REVIEW_INPUTS),
      clean = "mark_ready", blockers = "round_check", wrong_ac = "ac_decision", world_moved = "poll_mr")

state("round_check", script("scripts/review-rounds {ticket_id} {mr_iid}"),
      under_cap = "request_changes", capped = "capped")

state("request_changes", script("scripts/request-changes"),
      posted = "poll_mr")

state("mark_ready", script("scripts/mark-ready"),
      ready = "merge_gate", moved = "poll_mr")

# ── Merge (yours only) and ship ─────────────────────────────────────────

state("merge_gate", gate("Merge {repo}!{mr_iid} at {head_sha}?", body_file = "{review_md}"),
      merge = "merge", hold = "poll_mr")

state("merge", script("scripts/merge"),
      merged = "ship", refused = "merge_refused")

state("ship", wait(probe = "scripts/ship-check", every = "5m", timeout = "2h"),
      green = "shipped", red = "red", timeout = "red")

# ── Needs you ───────────────────────────────────────────────────────────

state("merge_refused",
      blocker("Merge refused: {refuse_reason}", fields = {"reason": "string?"}),
      retry = "poll_mr", override = "merge", stop = "failed")

state("red", blocker("Pipeline or rollout is red for {repo}"),
      retry = "ship", stop = "failed")

state("capped",
      blocker("Three review rounds on {repo}!{mr_iid} and one blocker still stands",
              body_file = "{review_md}"),
      rereview = "review", stop = "failed")

state("ac_decision", choice("An acceptance criterion looks wrong", body_file = "{review_md}"),
      amend = "failed", keep = "review")

state("stalled", blocker("No MR movement on {ticket_id} for 72h"),
      keep_waiting = "poll_mr", stop = "failed")

# ── Ends ────────────────────────────────────────────────────────────────

state("shipped", done("merged to {trunk}, pipeline green, live on {env}: go test"))
state("failed", fail())
