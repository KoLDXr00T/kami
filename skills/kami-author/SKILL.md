---
name: kami-author
description: Write or port a Kami workflow — the Starlark DSL (workflow, state, agent, script, wait, spawn, choose, ask/choice/form/blocker/gate/action), kekkai.yaml, the script and resolver protocols, and validate/dry-run. Use when the user says "write a kami workflow", "port this skill to kami", "add a step to <workflow>.star", or edits files under ~/.kami/kekkai.
---

# kami-author — workflows and domains

## Where things go

```
~/.kami/kekkai/<name>/
  kekkai.yaml        workers, skills, env file, cwd roots, AFK default
  env                KEY=VALUE — the ONLY env vars workers and scripts get (+ PATH, HOME, USER, LANG, TMPDIR)
  prompt.md          appended to every worker's system prompt
  skills/<s>/SKILL.md
  workflows/<wf>.star   file name must equal workflow(name = ...)
  lib/*.star         fragments, pulled in with load("lib/x.star", "fn")
  scripts/*          script steps and probes
  resolvers/*        AFK resolvers
```

```yaml
# kekkai.yaml
name: acme
description: Acme work
env_file: env
prompt: prompt.md
skills: [flow-review]                 # given to every agent step here
cwd_roots: [~/code/work]
workers:
  reviewer:       {provider: claude, model: claude-opus-5-5}
  reviewer-codex: {provider: codex}
  stub:           {provider: fake, command: scripts/stub.sh}   # tests
afk_default: {kind: attended}
```

## The DSL

One `workflow(...)` call, then one `state(...)` per state. Outcomes are
keyword arguments mapping to the next state. Logic goes in named `def`
functions.

```python
load("lib/multica.star", "enqueue")

workflow(
    name = "ticket",
    inputs = {"task": "dict", "trunk": "string", "lane": "string?"},  # ? = optional
    start = "file",
    afk = afk("grace", grace = "10m", resolver = "resolvers/judge"),  # default for decisions
    resume = "aura",            # or "session": a woken worker resumes its Claude/Codex session
    max_visits = 5,             # per state; exceeding it parks a blocker
    entries = {"followup": entry("plan", require = ["goal"])},        # kami run start ticket:followup
    triggers = [cron("0 9 * * 1-5", input = {"lane": "claude"})],     # 5 or 6 (seconds-first) fields
)

def reviewer(v):
    return "reviewer-codex" if v.get("lane") == "codex" else "reviewer"

state("file", script("scripts/file-ticket {trunk}"), filed = "review", duplicate = "stopped")
state("review",
      agent(reviewer, "Follow the flow-review skill.", skills = ["flow-review"],
            inputs = ["ticket_id", "trunk"], cwd = "{repo_path}", timeout = "30m"),
      clean = "merge_gate", blockers = "review")
state("merge_gate", gate("Merge {repo}!{mr_iid}?", body_file = "{review_md}"),
      merge = "shipped", hold = "review")
state("shipped", done("merged {repo}!{mr_iid}"))
state("stopped", cancel())
```

### Steps

| Step | Runs | Outcome |
|---|---|---|
| `agent(worker, instruction, skills=, inputs=, cwd=, timeout=1h, resume=)` | a worker; `worker` may be a function of vars | the worker's `kami step done --outcome` |
| `script(cmd, inputs=, cwd=, timeout=10m)` | a command, no shell, no LLM | last stdout line `{"outcome": "...", "vars": {...}}`; without JSON: `ok` on exit 0, `error` otherwise; `timeout` if killed |
| `wait(signal=, key=, timer=, probe=, every=, timeout=, max_errors=3)` | sleeps; a probe script runs every `every` | `signal`, `timer`, `timeout`, or the probe's first outcome other than `pending` |
| `spawn(workflow, for_each, input=)` | one child run per item of a list var (or `fn(vars)`); `input(item, vars)` builds its inputs (default `{"item": item}`) | `all_done` / `any_failed`; `vars.children[i]` = `{run_id, status, outcome, report, vars}` |
| `choose(fn)` | `fn(vars)` returns the next state name | — |
| `done(report)`, `fail(report)`, `cancel(report)` | ends the run | the terminal state's name is the run's outcome |

### Decisions

| Builder | Outcomes | Who resolves |
|---|---|---|
| `ask(title)` | `answered`; the text lands in `vars[<state>]` | user or resolver |
| `choice(title)` | the outcome names are the answers | user or resolver |
| `form(title, fields = {"rating": "enum:bad,good", "note": "string?"})` | declared outcomes; fields merge into vars | user or resolver |
| `blocker(title)` | the ways forward | user or resolver |
| `gate(title)` | approve something irreversible | **user only** |
| `action(title)` | the user does something outside Kami | **user only** |

All take `body=` (string or `fn(vars)`), `body_file=` (path; relative = the
run dir), `default=`, `resolver=`, `mode=` (`"afk"`, `"grace"` with
`grace="5m"`, or `afk(...)`), and `lock=True` (user only). A resolution
writes its answer to `vars[<state name>]` — name a state after the var you
want (`state("mood", ask(...))` → `vars["mood"]`).

### Rules that bite

- `{name}` / `{a.b}` in strings is filled from run vars at run time. Missing
  vars render empty (logged as `template.missing` for instructions and
  commands; a `cwd` or `body_file` with a missing var is an error). `{{` is a
  literal brace.
- Commands are split into words **before** rendering, and run without a
  shell, so a var can never inject arguments. Need a pipe? Write a script.
- A command whose first word has a `/` (`scripts/x`) resolves inside the
  kekkai; anything else is looked up on PATH.
- Functions see a frozen `vars` dict with `visits` (and `children` after a
  spawn) inside. No I/O, no clock, no randomness. Return string literals from
  `choose` functions so validation can follow every branch.
- `inputs=` is an allowlist: the worker prompt (and a script's stdin JSON)
  gets only those vars. Use it to keep a reviewer from seeing the author's
  reasoning.
- `cwd=` must resolve under a `cwd_roots` entry, checked at validation for
  literals and at run time for templates.
- `pending` is reserved for probes. A probe returning `pending` writes
  nothing to memento, so polling for hours is free. `{"outcome": "pending",
  "say": "CI 3/5 green"}` posts the say as a `run.say` line, only when it
  differs from that probe's last say.
- Every `wait` on a signal or probe needs `timeout=` and a `timeout` outcome.
- A probe that exits `error` (non-zero, no JSON) is retried with backoff
  (after `every`, then 2×, 4×, up to 8×); `max_errors` errors in a row
  (default 3) open a retry/stop blocker. `pending` resets the count. Map
  `error` to a state to handle it yourself.
- Scripts re-run after a daemon crash: make them check-first (did the ticket
  already get filed?) before mutating anything.

## Protocols

**Script/probe**: stdin = JSON of the step's vars; env = kekkai env +
`KAMI_RUN`, `KAMI_STEP`, `KAMI_STEP_DIR`, `KAMI_KEKKAI_DIR`, `KAMI_SOCK`;
last stdout line = `{"outcome": "x", "vars": {...}}`.

**Resolver**: stdin = `{"contract": {...}, "run": {"id", "workflow", "state", "vars"}}`;
last stdout line = `{"answer": "...", "data": {...}, "note": "..."}`. Exit
non-zero or a wrong answer → the contract goes back to `attended`.

**Worker**: see the `kami-worker` skill (injected into every agent step).

## Check before running

```sh
kami workflow validate <name|path.star>   # every edge, reachability, workers, skills, paths
kami workflow dry-run <name|path.star>    # the transition table, breadth first
kami kekkai validate <kekkai>             # the whole domain
```

`kami run start` refuses a workflow that does not validate. A run pins the
hash of its files (including `load`ed libs), so editing a workflow never
changes a run already in flight.

## Porting an existing skill

1. List the skill's phases, the decisions it asks the user, the commands it
   runs, and anything it polls.
2. Each deterministic command → `script` (write one entry point that prints
   the JSON outcome). Each judgment step → `agent` with the phase's prompt as
   a kekkai skill. Each question to the user → a decision builder; anything
   that pushes, files, merges or spends → `gate`.
3. "Run under /loop" → `wait(probe=..., every=...)`. "Hand the agent only X"
   → `inputs=[...]`. "One per item" → `spawn`. "Retry N times" → `max_visits`.
4. Credentials the skill reads from env go in the kekkai `env` file.
5. `kami workflow dry-run` it and read every edge against the original skill.
