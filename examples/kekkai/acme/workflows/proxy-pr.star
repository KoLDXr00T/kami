# proxy-pr: push your branch, a Multica agent opens the MR under its own identity.
# Start from the checkout: kami run start proxy-pr --input repo_path=$PWD

load("lib/multica.star", "enqueue")

workflow(
    name = "proxy-pr",
    inputs = {"repo_path": "string", "lane": "string?"},
    start = "preflight",
)

REPO = "{repo_path}"

state("preflight", script("scripts/proxy-preflight", cwd = REPO),
      ok = "audit", refuse = "refused")

state("audit",
      agent("comment-auditor", "Audit comments against origin/{trunk}.", cwd = REPO, inputs = ["trunk"]),
      clean = "go_ahead", edited = "go_ahead")

# One gate covers push, ticket and assign. gate_md carries any auditor edits as a diff.
state("go_ahead", gate("Push {branch} and file the MR ticket?", body_file = "{gate_md}"),
      go = "push", stop = "stopped")

state("push", script("scripts/push-verify", cwd = REPO),
      ok = "file", mismatch = "refused")

state("file", script("scripts/file-mr-ticket"),
      filed = "await_run", duplicate = "refused")

enqueue(then = "filed", on_dead = "refused")

state("filed", done("{branch} pushed at {head_sha}; {ticket_id} filed; run {run_status}"))
state("refused", fail("{refuse_reason}"))
state("stopped", cancel())
