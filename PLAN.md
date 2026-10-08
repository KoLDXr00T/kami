# Kami — Phase 1 plan

Kami (神) is a local, event-driven agent OS. A Go daemon runs workflows written in
Starlark. Each step runs a headless Claude Code or Codex worker, or a plain script.
Workers block on out-of-band contracts, sleep, and wake from a compact snapshot.

## Decisions (from the jane thread, 2026-10-07/08)

| Topic | Decision |
|---|---|
| Codebase | Greenfield Go. Krakoa (`~/code/krakoa`) is a reference only |
| First workflows | Synthetic test workflows that exercise every mechanism. Real domains get ported later (dry runs of three are below) |
| Workflow language | Starlark (`go.starlark.net`): a declarative state machine with pure functions for guards and transitions |
| Worker (Shikigami) | Headless Claude Code (`claude -p`) or Codex (`codex exec`), chosen per worker |
| Wake-up | Rehydrate from the Aura snapshot by default. A workflow can opt into full session resume |
| Durable state | memento namespace `kami`: runs, contracts, Aura snapshots (one commit per write is accepted) |
| Event log | Local SQLite `~/.kami/events.db` (high frequency, append-only) |
| Contract surface | Terminal (`kami contract list/resolve`) plus an event stream (`kami events tail -f`). jane, phoenix and Slack adapters come later and consume that stream |
| AFK | Each contract has a resolver (script, LLM judge, codex, jev: any command) and a mode: `attended`, `grace(d)`, `afk` (grace 0). The workflow sets a default. It can be overridden per run (at start or live) and per open contract (live) |
| Kekkai | Process-level isolation: separate config, skills and env per domain, same macOS user. "Active" Kekkai = the CLI default only (like a kubectl context). All domains keep running |
| Event sources | The `kami` CLI (`kami signal pub`) and timers (waits and cron triggers) |
| Runtime | Local daemon on this Mac (launchd), unix socket only |
| Presentation | Terminal only |

## Repository layout (as built)

The spec's `core/*`, `cli/` and `sdk/harness/` map to Go's `cmd/` + `internal/`. The
plan had separate `contract/`, `harness/` and `daemon/` packages. They share one lock
and one run model, so they ended up as files of a single `engine` package.

```
kami/
  go.mod
  cmd/kami/                 # one binary: CLI subcommands + `kami daemon`
    main.go                 #   daemon client commands (run, contract, afk, signal, events, worker side)
    local.go                #   daemon lifecycle, kekkai and workflow commands (no daemon needed)
  internal/
    kamui/                  # Kamui: Starlark builtins, loader (+ load() bundles), validator, templates, cron
    eventbus/               # events.db (append-only) + live subscribers for `events tail -f`
    kekkai/                 # domain config, env files, cwd roots, worker env, active context
    state/                  # memento client (`memento --json db exec/query`), schema, bundles
    engine/                 # the daemon core
      engine.go             #   runs, transitions, spawn, cancel, boot recovery, timers
      harness.go            #   Shikigami: claude/codex/fake workers, scripts, prompts, worker callbacks
      contracts.go          #   contract queue, AFK modes, locks, resolvers, live overrides
      waits.go              #   timers, probes, signals, cron triggers
      api.go                #   HTTP API over the unix socket, views
      model.go              #   Run, Contract, Aura, ids
  skills/                   # kami, kami-author, kami-worker (embedded into every worker)
  e2e/                      # the done check: real daemon, fake workers, temp homes and stores
    testdata/kekkai/        #   test, other, cron domains and their workflows
  examples/kekkai/          # dry-run workflows for acme and suno (see Dry runs)
  deploy/ai.kami.daemon.plist
  Makefile                  # build, test, race, smoke, bench, install, launchd-install
```

Dependencies: `go.starlark.net`, `modernc.org/sqlite` (pure Go, same as memento),
`gopkg.in/yaml.v3`. CLI parsing is a small hand-written parser, because stdlib `flag`
stops at the first positional argument.

## On-disk layout

```
~/.kami/
  config.yaml               # active kekkai, default memento namespace
  kami.sock                 # daemon API
  events.db                 # event log (local SQLite)
  kekkai/<name>/
    kekkai.yaml             # KekkaiConfig
    env                     # dotenv: the only env vars workers in this domain get
    prompt.md               # system prompt appended for every worker in this domain
    skills/<skill>/SKILL.md
    workflows/*.star
    scripts/*               # script-step commands
    resolvers/*             # AFK resolver commands (scripts, judge wrappers)
  runs/<run-id>/<step>/     # worker cwd (unless the step sets cwd) + transcripts
```

## Skills

Three Claude Code skills live in `skills/`. `make install` symlinks them into
`~/.claude/skills/`, so the repo stays the source of truth.

| Skill | Who uses it | What it covers |
|---|---|---|
| `kami` | You, in any Claude Code session | Start runs (`kami run start`), list and resolve contracts, set AFK on a run or contract, read `kami run show` / `kami events tail`, switch Kekkai. Triggers: "kami", "start a kami run", "what's waiting on me", "afk this run" |
| `kami-author` | You or Claude, writing workflows | The Starlark DSL reference, `kekkai.yaml` fields, `kami workflow validate` / `dry-run`, and a porting checklist that turns an existing skill (phases, gates, scripts) into a workflow. Triggers: "write a kami workflow", "port this skill to kami" |
| `kami-worker` | Every Shikigami, injected by the harness | The worker protocol: finish with `kami step done --outcome`, raise `kami contract request` and `kami state snapshot` then exit when blocked, never ask in chat, read the Aura and the resolution on wake |

`kami-worker` is not optional. Without it a worker has no way to report an outcome.
The harness always adds it to the step's skills, whatever the Kekkai lists.

## Workflow DSL

A workflow is a `.star` file. It calls `workflow(...)` once, then `state(...)` once per
state, the way a Bazel `BUILD` file declares one target per rule call. Logic lives in
named `def` functions. Shared fragments come in with `load("lib/x.star", "fn")` from
the Kekkai's `lib/`; a fragment is a function that declares several states.

```python
load("lib/multica.star", "enqueue")

workflow(name = "ticket", inputs = {"task": "dict", "trunk": "string"}, start = "file")

def reviewer(v):
    return "reviewer-codex" if v.get("lane") == "codex" else "reviewer"

state("file", script("scripts/file-ticket"),
      filed = "await_run", adopted = "await_run")

enqueue(then = "poll_mr", on_dead = "lane")

state("review",
      agent(reviewer, "Follow the flow-review skill.", skills = ["flow-review"],
            inputs = ["ticket_id", "repo", "mr_iid", "head_sha", "trunk"]),
      clean = "mark_ready", blockers = "round_check")

state("merge_gate", gate("Merge {repo}!{mr_iid} at {head_sha}?"),
      merge = "merge", hold = "poll_mr")
```

`state(name, step, **outcomes)`: each keyword maps an outcome to the next state.
Outcomes are snake_case. For decisions, the outcome names are also the options you
pick from.

### Steps

| Step | Runs | Outcome comes from |
|---|---|---|
| `agent(worker, instruction, skills=, inputs=, cwd=, timeout=, resume=)` | a Claude or Codex worker; `worker` can be a function of vars | `kami step done --outcome X` |
| `script(cmd, inputs=, cwd=, timeout=)` | a command, no LLM | stdout JSON `{"outcome", "vars"}`; no JSON means `ok` on exit 0, `error` otherwise |
| `wait(signal=, key=, timer=, probe=, every=, timeout=)` | sleeps, or runs `probe` every `every` | `signal`, `timer`, `timeout`, or the first probe outcome other than `pending` |
| `spawn(workflow, for_each, input)` | one child run per item; `input(item, vars)` builds its inputs | `all_done` or `any_failed`. `vars.children` holds each child's `outcome` (its terminal state name), `report` and `vars` |
| `choose(fn)` | a pure function | the state name `fn(vars)` returns |
| `done(report)`, `fail(report)`, `cancel()` | nothing | ends the run with that status |

### Decisions (contracts)

| Builder | Asks | Who can resolve |
|---|---|---|
| `ask(title)` | a free-text question; outcome `answered` | you, or a resolver per AFK mode |
| `choice(title)` | pick one outcome | you, or a resolver |
| `form(title, fields=)` | fill in fields; outcomes as declared | you, or a resolver |
| `blocker(title)` | something is broken; outcomes are the ways forward | you, or a resolver (`ccpush`, `acme-init`) |
| `gate(title)` | approve an irreversible step: push, file, merge | **you only**; AFK never applies |
| `action(title)` | do something outside Kami, then resolve (generate on Suno) | **you only** |

All six take `body=` (a string or a function of vars), `body_file=`, `resolver=`,
`mode=`, and `lock=True` to make any of the first four human-only. A resolution
writes its answer into `vars[<state name>]` and merges its fields into vars, so
`state("mood", ask(...))` leaves the answer in `vars["mood"]`.

### Rules

- `{name}` inside strings is filled from run vars when the step runs. It is Kami's
  templating, not Starlark's (Starlark's `%` and `.format()` run at load time).
- Functions get a read-only `vars` dict with `visits` and `children` inside it.
  Starlark gives them no file access, clock or randomness.
- `inputs=` is an allowlist. The worker's prompt gets only those vars.
- `cwd=` must sit under one of the Kekkai's `cwd_roots`.
- `workflow(...)` also takes `afk=`, `resume=`, `max_visits=` (default per state; steps
  can override), `entries = {"name": entry(state, require = [...])}` and
  `triggers = [cron("0 9 * * *")]`.

`kami workflow validate` checks: start state and entries exist, no duplicate states,
every outcome targets a state, every state is reachable, every wait has a timeout,
every `cwd` sits under a Kekkai root.

## Domain schemas

All IDs are ULIDs. All timestamps are RFC 3339 UTC.

### ContractRequest

```json
{
  "id": "01J...",
  "run_id": "01J...",
  "step": "draft",
  "kekkai": "test",
  "origin": "state | worker",
  "kind": "ask | choice | form | blocker | gate | action",
  "title": "Approve the draft?",
  "body": "markdown context for whoever resolves it",
  "options": ["approve", "reject"],
  "fields": [{"name": "reason", "type": "string", "required": false}],
  "default": "approve",
  "lock": false,
  "mode": {"kind": "attended | grace | afk", "grace": "10m"},
  "resolver": {"command": "resolvers/auto-approve", "timeout": "5m"},
  "status": "open | resolved | cancelled",
  "deadline": "2026-10-08T12:10:00Z",
  "created_at": "..."
}
```

- `origin: state` means the workflow declared a contract state. `origin: worker` means
  a worker raised it mid-step through `kami contract request` and is now asleep.
- `kind: action` asks a human to do something outside Kami and resolve when done
  (for example, generate a song on Suno).
- `deadline` is stored, not computed, so grace timers survive a daemon restart.

### ContractResolution

```json
{
  "contract_id": "01J...",
  "answer": "approve",
  "data": {"reason": "looks fine"},
  "by": "human | resolver",
  "resolver": "resolvers/auto-approve",
  "note": "free text",
  "resolved_at": "..."
}
```

The first resolution wins (`UPDATE ... WHERE status='open'`). A human can answer during
the grace window, and then the resolver never runs. If the resolver fails or returns an
invalid answer, the contract switches to `attended`. It is never silently dropped.

Resolver protocol: Kami runs the resolver command with the ContractRequest JSON on
stdin and the Kekkai env. The command writes `{"answer", "data", "note"}` to stdout.
That protocol covers LLM judges, codex, jev and plain scripts.

### AuraSnapshot

```json
{
  "run_id": "01J...",
  "step": "draft",
  "seq": 3,
  "goal": "what this step is trying to achieve",
  "progress": ["done so far, one line each"],
  "next": "what to do on wake",
  "vars": {"draft_path": "runs/01J.../draft/out.md"},
  "waiting_on": "contract:01J...",
  "session": {"provider": "claude", "id": "uuid", "resume_allowed": false},
  "created_at": "..."
}
```

On wake, the harness starts a fresh worker. Its prompt is the step instruction, the
latest Aura and the contract resolution. If the workflow sets `resume = "session"`,
the harness runs `claude --resume <id>` instead.

### KekkaiConfig (`kekkai.yaml`)

```yaml
name: acme
description: Acme work — tickets, MRs, rollouts
env_file: env                  # only these vars + PATH/HOME reach workers
prompt: prompt.md
skills: [flow-refine, flow-ground, flow-review, flow-conventions]
cwd_roots: [~/code/work]   # where steps may set cwd
workers:
  refiner:  {provider: claude, model: claude-opus-5-5}
  reviewer: {provider: claude}
  reviewer-codex: {provider: codex}
afk_default: {kind: attended}
memento_namespaces: [acme]   # declared, not enforced (see Not handled yet)
```

### KamuiState (run record, memento table `runs`)

```json
{
  "id": "01J...",
  "workflow": "draft-review",
  "def_hash": "sha256 of the .star file (a run pins its definition)",
  "kekkai": "test",
  "parent": null,
  "entry": "start",
  "state": "review",
  "status": "running | sleeping | waiting | done | failed | cancelled",
  "vars": {"topic": "x"},
  "visits": {"draft": 1, "review": 1},
  "afk_override": null,
  "wake_at": null,
  "updated_at": "..."
}
```

## CLI

Every command takes `--json` and returns `{ok, data, error, meta}` (same envelope as
memento). Exit codes are stable per error class.

```
kami daemon [--foreground]                       run the daemon (launchd uses this)
kami status                                      daemon, stores, open contracts, sleeping runs

kami kekkai list | current | switch <name> | validate [<name>]

kami workflow list | validate <file> | dry-run <file>   # dry-run walks every edge synthetically
kami run start <workflow>[:<entry>] [--kekkai K] [--input k=v]... [--afk attended|grace=10m|afk]
kami run list [--status S] [--var k=v] | show <id> | cancel <id>

kami signal pub <type> [--key K] [--data JSON] [--kekkai K]

kami contract list [--status open] [--kekkai K] | show <id>
kami contract resolve <id> <answer> [--data JSON] [--note T]
kami afk show <run-id>
kami afk set <attended|grace=D|afk> (--run <id> | --contract <id>)   # refused on locked contracts

kami events tail [-f] [--since <id>] [--type T] [--kekkai K]   # NDJSON; future adapters read this

# called by workers (the harness sets KAMI_RUN, KAMI_STEP, KAMI_TOKEN)
kami step done --outcome <o> [--vars JSON]
kami contract request --kind K --title T [--options a,b] [--default a]   # then exit = sleep
kami state snapshot < aura.json
kami state show <run-id>
```

## Write ordering and recovery

memento is the source of truth. The daemon is the only writer to the `kami` namespace
and serialises its writes on one goroutine. Every transition writes memento first, then
appends the event to `events.db`. On boot, the daemon:

1. loads runs that are not terminal
2. re-arms `wake_at`, contract `deadline` and cron timers
3. re-spawns steps that were `running` when it died, as a wake from the latest Aura.
   Scripts are re-run, so script steps must be safe to repeat (check-first)

Events are a log, never replayed for state, so losing the last event after a crash
loses nothing.

## Dry runs of existing skills

Three real skill sets, walked through Kami on paper. The workflows are real `.star`
files in `examples/kekkai/`. They parse in `go.starlark.net`, and every outcome targets a
state that exists (checked with stub builtins, since the engine isn't built yet).
Script names are placeholders for what the skills do in prose today.

| File | What it is |
|---|---|
| `acme/lib/multica.star` | `preflight` and `enqueue` fragments, shared by `goal`, `ticket` and `proxy-pr` |
| `acme/workflows/goal.star` | flow start + followup: refine, ground, dry-run, dispatch gate, one `ticket` per task |
| `acme/workflows/dryrun-review.star` | one task's text reviewed cold before filing |
| `acme/workflows/ticket.star` | file, enqueue, poll MR, review loop, merge gate, ship |
| `acme/workflows/proxy-pr.star` | preflight, comment audit, one gate, push, file, enqueue |
| `suno/workflows/song.star` | write, mood question, lint, you generate, download, analyze, you rate |
| `suno/workflows/analyze-clip.star` | one clip through the muz analyzer |
| `suno/workflows/suno-sync.star` | daily cron import of songs made outside a run |

### 1. acme `flow` (idea → ticket → build → review → merge → shipped)

Source: `~/code/work/.claude/skills/flow/` (SKILL.md, prompts
refine/ground/review, reference dispatch/merge/conventions, scripts queue/flowlog/await_run).

Three workflows: `goal`, `dryrun-review` (one child per task) and `ticket` (one child
per filed task). `refine` reports `tasks`, `goal` (the kebab-case goal slug) and
`trunk` through `kami step done --vars`.

How each flow phase maps:

| flow phase | In Kami | Status |
|---|---|---|
| start: refine → questions | `agent` + `ask`; the answer lands in `vars.answers` for the next refine | covered |
| start: ground, 3-round cap | `agent` with `inputs=` allowlist, `max_visits = 3` | covered (needs `inputs=`) |
| start: dry-run review per task, in parallel | `spawn` of `dryrun-review` children | covered (needs `spawn`) |
| start: dispatch after your go | `gate`, then `spawn("ticket")` | covered |
| dispatch: await_run, rerun only on `none` | `enqueue` fragment in `lib/multica.star` | covered (needs `script`) |
| auth-failed → `ccpush` | `blocker` contract in `afk` mode, resolver `ccpush` | covered |
| 401 → `/acme-init` | `blocker` contract with a resolver | partly: `/acme-init` is a skill, it needs a script form to be a resolver |
| status | `kami run show` + the current state | covered |
| review (pinned 5-tuple) | `agent` with exactly 5 inputs | covered |
| sweep under `/loop 10m` | each ticket run polls its own MR (`wait(probe=...)` every 10 min). No token cost while idle | covered, replaces `/loop` |
| request changes, mark ready | `script` states, no gate (as sweep does today) | covered |
| round cap (3 reviews) | `round_check` script counts `## Review — !iid` comments | covered |
| merge, user only | `gate`; the merge guards stay in `scripts/merge` | covered |
| merge override with a reason | `override` outcome with a `reason` field on `merge_refused` | covered |
| ship (pipeline + rollout) | `wait(probe=...)` every 5 min; `canceled` reads as `pending` in the script | covered |
| followup onto the role's ticket | `goal:followup` entry; comment + status flip as scripts | partly: the goal run is already terminal, so the followup has to start a new run seeded with `goal`. A ticket run waiting in `poll_mr` can't be reused |
| flowlog events + `trace` | Kami event log + `kami run show` | partly: `flow-analytics/mine.py` reads `flowlog`'s `events.db` and would need porting |
| Claude/Codex lane | `reviewer(v)` picks the worker; the `lane` choice rewrites `vars.lane` | covered |

One behaviour change: flow today runs one phase per command and never chains. In Kami,
the run chains on its own and stops only at contracts. The locked gates sit where flow
asks for your go today: dispatch and merge. Ready-flips and request-changes run
without asking, as sweep already does.

### 2. acme `proxy-pr` (push the branch, a Multica agent opens the MR)

Source: `~/code/work/.claude/skills/proxy-pr/SKILL.md`.

Started from your working session: `kami run start proxy-pr --input repo_path=$PWD`.

| proxy-pr step | In Kami | Status |
|---|---|---|
| preflight refusals (trunk branch, dirty tree, `CAL-` ids, existing MR) | `script` with a `refuse` outcome | covered |
| commit/diff review, flags | same preflight script writes them into `gate_md` | covered |
| comment-auditor pass, edits shown at the gate | `agent` with `cwd = {repo_path}`; the diff goes into `gate_md` | covered (needs `cwd=`) |
| one gate covers push + ticket + assign | one `gate` | covered |
| push, verify remote head | `script` | covered |
| file ticket, duplicate check, assign, await_run, rerun, ccpush | `script` + the shared `enqueue` fragment | covered |
| report | `done(report)` | covered |

proxy-pr fits Kami with no gaps beyond the primitives added above.

### 3. muz: Suno song pipeline

Sources: `~/music/CLAUDE.md` (song file rules), the `suno-prompt` skill,
`suno-download`, `music-analyze`, the muz `Makefile` (`lint`, `analyze`).

| Suno step | In Kami | Status |
|---|---|---|
| neutral-mood gate ("name the mood in one word or ask") | worker outcome `no_mood` → `ask`; the answer lands in `vars.mood` | covered |
| write `songs/<title>.txt`, never overwrite (`_v2`) | `agent` with `cwd = ~/music`; the rule stays in CLAUDE.md | covered (needs `cwd=`) |
| 1000-char style cap, BPM floor | `make lint` as a `script` that loops back to `write` | covered |
| generate on Suno | `action`. Suno has no API, so this is always you | covered, always manual |
| download (CDP Chrome capture, R2 mirror, memento `music` rows) | `wait(probe=...)` wrapping the four skill scripts; `none` new clips = `pending` | partly: `suno-download` is written as agent instructions; it needs a single script entry point |
| Suno makes two clips per prompt | `spawn("analyze-clip")` per clip | covered (needs `spawn`) |
| analysis (2–5 min per track) | `script` with a 20-minute timeout | covered |
| rating | `form(..., lock = True)` with fields | covered |
| "new concept, not a re-roll" after a bad track | `choose(verdict)` → a `form` asking for a new concept | covered |
| tune the next prompt from analysis numbers | `analysis` passed into `write` via `inputs=` | covered |

## Coverage: handled vs not yet

The dry runs needed eight additions to the first draft of this plan. They are now part
of the DSL above and of the milestones below:

1. `script` steps: deterministic commands, no LLM. All three skill sets use them more
   than agents.
2. `inputs=` allowlists per step: flow's isolation rule enforced by the engine.
3. `spawn` child runs with wait-all: flow's per-task fan-out, Suno's two clips.
4. `entry` points: flow's followup.
5. Human-only decisions: `gate` and `action`, and `lock=True` on the others (dispatch, merge, push, rate).
6. `cwd=` with Kekkai `cwd_roots`: proxy-pr and Suno run in real checkouts.
7. `cron` triggers and a dynamic worker function: `suno-sync`, flow's Claude/Codex lane.
8. `wait(probe=, every=)`: polling inside one state. A probe returning `pending` writes
   nothing, so a ticket waiting on its builder for a day makes no memento commits.

### Not handled yet

- **memento namespace scoping is not enforced.** Workers run as the same user with the
  same `MEMENTO_STORE`, so `memento_namespaces` in `kekkai.yaml` is only documentation.
  Enforcing it needs a memento-side token or a per-Kekkai store.
- **Credentials in HOME are shared across Kekkai.** `glab`, `multica`, `kubectl` and
  `aws` read their config from HOME, not env. Light isolation keeps env separate, not
  those files. Light isolation was chosen, so this is expected; it is listed so it isn't
  forgotten.
- **Skills written as prose need script entry points before they can be steps.**
  `suno-download`, `/acme-init`, flow's dispatch and merge recipes (`reference/*.md`)
  are instructions to an agent today. Each needs a script with JSON output to run as a
  `script` step, or it runs as an `agent` step at token cost.
- **A followup can't wake a finished run.** Kami starts a new run seeded with the goal
  slug. Fine for flow; noted in case another domain needs to reopen runs.
- **flow analytics.** `flow-analytics/mine.py` reads `flowlog`'s `events.db`. Porting
  it to Kami's event log is part of the acme migration, not Phase 1.
- **Webhooks.** Every external wait above polls on a timer (MR state, pipeline, Suno
  feed). That works and costs no tokens, but adds up to 10 minutes of latency.
  Webhooks are out of Phase 1.

## Test workflows (Phase 1 "done" check)

They live in `e2e/testdata/kekkai/{test,other,cron}`. A `fake` worker provider runs a shell
script that calls the `kami` CLI, so `go test` needs no LLM. Tests set `MEMENTO_STORE`
to a temp dir and never touch the real store.

| # | Workflow | Proves |
|---|---|---|
| 1 | `linear` | agent → agent → terminal; outcomes route transitions |
| 2 | `gate` | contract state; `kami contract resolve` picks the branch |
| 3 | `blocker-wake` | worker raises a contract mid-step, writes an Aura and exits. After resolve, a new worker gets only the Aura and finishes |
| 4 | `afk-grace` | (a) no human answer in 2s → resolver answers; (b) human answers in time → resolver never runs |
| 5 | `afk-full` | grace 0 → resolver immediately; a failing resolver falls back to attended |
| 6 | `afk-override` | live `kami afk set` on a run and on an open contract changes the outcome |
| 7 | `signal-wait` | wait matched by correlation key; `kami signal pub` wakes the right run only; timeout arm fires |
| 8 | `timer` | timer wait fires after 2s |
| 9 | `crash-recovery` | kill the daemon mid-grace and mid-wait, restart; deadlines honoured, runs finish |
| 10 | `kekkai-isolation` | worker in `test` cannot see `other`'s env; switching active Kekkai changes CLI defaults only |
| 11 | `loop-budget` | exceeding `max_visits` raises a blocker contract instead of looping |
| 12 | `script-step` | script JSON outcome and vars route; non-zero exit without JSON routes `error` |
| 12b | `probe-wait` | a probe returns `pending` twice (no memento writes), then `ok` routes; timeout arm fires |
| 13 | `inputs-scope` | a worker's prompt contains only the vars named in `inputs=` |
| 14 | `spawn` | three children, one fails → `any_failed`, `vars.children` holds all three outcomes |
| 15 | `entry` | `run start wf:followup` starts at the entry state with seed vars; missing `require` is refused |
| 16 | `lock` | `kami afk set --run` and `--contract` leave a locked contract attended; the resolver never runs |
| 17 | `cwd-roots` | a `cwd` outside `cwd_roots` fails validation; one inside runs there |
| 18 | `cron` | a cron trigger starts a run; after a daemon restart the trigger fires again |
| 19 | `session-resume` | `resume = "session"` resumes the real Claude session (manual `make smoke`, uses tokens) |

Phase 1 is done when `go test ./...` passes tests 1–18 and `make smoke` passes 19 plus
one real-Claude run of `blocker-wake`.

## Milestones

Each milestone ends with `go test ./...` green.

| M | Build | Check |
|---|---|---|
| 0 | `go.mod`, `cmd/kami`, daemon on unix socket, `kami status` | daemon starts, `kami status --json` ok |
| 1 | `state` (memento client, `kami` ns migrations) + `events.db` | round-trip tests; benchmark of 200 contract writes through memento, p50/p95 reported |
| 2 | `kekkai`: load, validate, switch, worker env builder, `cwd_roots` | tests 10 (env half), 17 |
| 3 | `kamui`: Starlark loader, validator, interpreter (`state`, steps, decisions, `load`), `workflow validate/dry-run` | interpreter unit tests, tests 11, 12 |
| 4 | `eventbus`: signals, waits, timers, probes, cron triggers, `events tail -f` | tests 7, 8, 12b, 18 |
| 5 | `contract`: queue, modes, locks, resolver runner, `afk set` | tests 2, 4, 5, 6, 16 |
| 6 | `harness`: fake, claude, codex providers; `inputs=`; `step done`, `state snapshot`, rehydrate | tests 1, 3, 13, then smoke 19 |
| 7 | `spawn` children, `entry` points | tests 14, 15 |
| 8 | recovery on boot + launchd plist | test 9 |
| 9 | skills `kami`, `kami-author`, `kami-worker`; `make install` | `kami workflow validate` passes every file in `examples/kekkai/` |

## Status (2026-10-08)

Every milestone M0–M9 is built. The done check passes:

- `make test`: unit tests plus 20 e2e tests (the 18 workflows above, plus a probe-wait
  commit count and a worker that never reports). Each test runs a real daemon against
  its own temp `KAMI_HOME` and memento store.
- `make race`: the same suite, with the daemon built and run under the race detector.
  No races were reported. Run 3x without flakes.
- `make smoke`: a real Claude worker (claude-haiku-5-5) raised a contract, saved an
  Aura, slept, and woke with the answer. A second worker with `resume = "session"`
  resumed its Claude session and still remembered a word from before the sleep. A
  real Codex worker passed the same blocker-wake flow. About 15 s each for Claude,
  35 s for Codex.
- M1 benchmark (`make bench`): 200 contract writes through memento, p50 60 ms, p95
  65 ms, max 74 ms. Each write re-dumps the namespace, so this grows with the number
  of rows in `kami`. Probe waits write nothing while pending.

Verified against the real CLIs: `claude -p --output-format json --setting-sources project
--plugin-dir --append-system-prompt --resume` (2.1.294) and `codex exec --json --cd
--ignore-user-config --ignore-rules`, `codex exec resume` (0.161.0).

Not installed: nothing was written to `~/.local/bin`, `~/.claude/skills` or launchd.
`make install` links the binary and skills; `make launchd-install` loads the daemon.

## Open / unverified

- unverified: whether skills from `--plugin-dir` load in `claude -p`. The worker
  protocol is also written into every Claude/Codex prompt, so the smoke runs did not
  depend on it. Kekkai skills (flow-review etc.) do depend on it; check with a real
  domain before relying on them.
- unverified: `--allowedTools` with a comma-joined list (only used when a worker sets
  `tools:`). The default is `--dangerously-skip-permissions`.
- A worker can reach the main socket. `contract resolve`, `afk set`, `run start` and
  `run cancel` refuse requests carrying a worker token, but a worker that unsets
  `KAMI_TOKEN` could still call them. That is the same-user limit of light isolation.
- After a crash, a worker process from before the crash may keep running. Its
  `kami step done` is rejected (stale token), but side effects it already made stay.
  This is why scripts and workers must check first.
- Cron fires missed while the daemon was down are skipped, not caught up.
- The `kami` memento namespace is created with `CREATE TABLE IF NOT EXISTS`, not
  memento migrations. A schema change later needs a migration.
- How jev is invoked as a resolver. The stdin/stdout resolver protocol should cover it
  through a wrapper script.
