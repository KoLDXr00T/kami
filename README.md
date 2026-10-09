# Kami 神

A local, event-driven agent OS. One Go binary, `kami`, runs a daemon that
executes workflows written in Starlark. Their steps are headless Claude Code or
Codex workers, or plain scripts. When a worker hits a decision it cannot make,
it does not ask in chat: it posts a **contract**, saves a compact **Aura**
snapshot, and exits. The run sleeps until you (or an AFK resolver) answer, and
then a fresh worker continues from the Aura.

| Name | What it is |
|---|---|
| Kamui | the control plane: loads `.star` workflows, validates them, moves runs between states. Code, not an LLM |
| Shikigami | a worker: `claude -p`, `codex exec`, or a fake script in tests |
| Contract | a decision in the queue: `ask`, `choice`, `form`, `blocker`, `gate`, `action` |
| AFK | per decision: `attended` (you), `grace=10m` (you, then a resolver), `afk` (resolver now) |
| Kekkai | a domain (`~/.kami/kekkai/<name>/`): its own env, prompt, skills, workflows, scripts |
| Aura | a worker's snapshot before it sleeps: goal, progress, next, vars |

Durable state (runs, contracts, Auras, pinned workflow files) lives in the
memento namespace `kami`. The event log is `~/.kami/events.db`.

## Build and run

```sh
make build                  # bin/kami
make install                # ~/.local/bin/kami + skills symlinked into ~/.claude/skills
make launchd-install        # keep `kami daemon` running (log: ~/.kami/daemon.log)
kami daemon start           # or run it by hand
kami status
```

Needs `memento` on PATH with an initialised store, and `claude` / `codex` for
real workers.

## A first workflow

```
~/.kami/kekkai/home/
  kekkai.yaml
  workflows/hello.star
```

```yaml
name: home
workers:
  writer: {provider: claude, model: claude-sonnet-5-5}
```

```python
workflow(name = "hello", inputs = {"topic": "string"}, start = "draft")

state("draft", agent("writer", "Write three lines about {topic} to draft.md, then report its absolute path as draft_path."),
      done = "review")
state("review", choice("Keep the draft?", body_file = "{draft_path}"), keep = "kept", redo = "draft")
state("kept", done("kept {draft_path}"))
```

```sh
kami kekkai switch home
kami workflow dry-run hello
kami run start hello --input topic=tea
kami contract list
kami contract resolve <id> keep
kami run show <run>
```

The DSL reference is in [`skills/kami-author/SKILL.md`](skills/kami-author/SKILL.md),
the operator's guide in [`skills/kami/SKILL.md`](skills/kami/SKILL.md), and the
worker protocol in [`skills/kami-worker/SKILL.md`](skills/kami-worker/SKILL.md).
Ported dry runs of real skills (acme flow, proxy-pr, Suno) are in
[`examples/kekkai/`](examples/kekkai/). The design and status are in
[`PLAN.md`](PLAN.md).

## Slack

`kami-slack` is Kami's Slack surface. It uses niffty's Slack app (it reads
`SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN` and `NIFFTY_OWNER_ID` from
`~/.local/share/niffty/.env`) and replaces niffty: keep niffty stopped while it
runs, or both will receive the same Socket Mode events.

```sh
make adapters                                   # bin/kami-slack (make install copies it to ~/.local/bin)
kami-slack serve [--poll 5s] [--kekkai K]       # contracts → owner DMs; other DMs → chat runs
      [--chat-kekkai work] [--chat-workflow chat]
kami-slack dm "build finished"                  # or text on stdin; prints {"channel","ts"}
kami-slack dm --thread <ts> "done"              # into a thread of the owner DM
kami-slack canvas --key weekly "Weekly" < r.md  # private canvas shared with the owner; prints its URL
```

`serve` DMs every contract to the owner as it opens. A choice-like contract
(options, no fields) gets one button per answer; tapping one resolves it and
redraws the message with the answer. An `ask` or `form` is answered by replying
in its thread: any text for an ask, `answer field=value …` for a form (values
parse as JSON when they can). Only the owner's taps and replies count. Kami's
refusal comes back in the thread; a contract resolved elsewhere, cancelled, or
handed back by a failed resolver gets a follow-up.

A run lives in a Slack thread: its var `thread_ts`, or the nearest ancestor's
(walking `parent`). A contract of such a run is posted into that thread (same
text and buttons; its follow-ups go there too), and the thread gets one line
per milestone: `run.say` → its text, a child's `run.started` →
`↳ started <workflow>`, `run.finished` → `✅ <workflow>: <report>` when done,
`❌ <workflow> <status>: <report>` otherwise. A run with no thread gets no
milestones and its contracts are top-level DMs as before. Milestones more than
10 minutes old (replayed after downtime) are not posted. A `thread_ts` that Slack refuses
(`invalid_thread_ts`) is treated as no thread: the contract goes top-level and
the milestone is dropped, recorded as an `error` entry.

An owner reply in a thread answers, in order: the contract whose top-level
message the thread hangs under; else the one open contract of the thread's
runs that takes text (an `ask`, from a state or a worker, a `form`, or a contract whose options
include the reply, case-insensitive). No such contract, or more than one, and
the reply is chat.

Every other owner DM is chat. It gets a 👀 and is keyed by its thread (a
top-level DM is the root of its own thread). It is published as a signal
`slack.message` with that key in the chat kekkai (`data: {text, ts, history}`),
for a chat run waiting on it. If none waits but an active run of the chat
workflow has `thread_ts` = key, the run is busy: the message is queued and
re-published, oldest first, on each poll until a run takes it. With no run at
all, a new `chat` run starts with inputs `thread_ts`, `channel`, `text` (all
queued messages), `history` (the thread so far as `owner:` / `bot:` lines,
last ~8000 characters; empty for a top-level DM) and `thread_runs` (the ids of
the runs tied to the thread, comma-separated). A run that cannot start posts
Kami's error in the thread. `--chat-workflow ""` turns chat routing off.

State (last event id, contract ↔ message, queued chat messages) is
`$KAMI_HOME/adapters/slack.json`; a canvas key's id is
`$KAMI_HOME/adapters/slack-canvas/<key>`, so the same key edits the same canvas.

### Audit

`serve`, `dm` and `canvas` append one NDJSON line per Slack message in or out
to `$KAMI_HOME/adapters/slack-transcript.ndjson` (mode 0600): `logged_at`,
`dir` (`in`/`out`), `kind` (`dm`, `thread_reply`, `button`, `reaction`,
`canvas`, `contract`, `followup`, `milestone`, `chat_routed`, `error`), `channel`, `ts`,
`thread_ts`, `user` (inbound), the full `text`, and when known `contract_id`,
`run_id`, `workflow`, `canvas_url`, `error`. A button tap has its `value` and
the `answer` Kami recorded; `chat_routed` says whether the message was
`signalled` (with the `delivered` run ids), `queued`, or `started` a run (its
`run_id`). A `dm` or `canvas` from a script step carries the step's `KAMI_RUN`.
Tokens and HTTP headers are never written.

```sh
kami-slack log [-f] [--run R] [--contract C] [--thread T] [--json]  # the transcript, filtered; -f follows
kami-slack threads [--since 24h] [--json]      # threads: root ts, first line, messages, last activity, runs, contracts
kami-slack thread <ts> [--json]                # one thread live from Slack, oldest first, with its contract/run ids
```

`thread` needs the Slack credentials; `log` and `threads` read only the file.

The Kami side of a run:

- `kami run show <run>`: its events, vars and contracts.
- `~/.kami/runs/<run>/<state>.<visit>.<attempt>/`: one directory per step
  attempt, with `prompt.md`, `stdout.log` and `stderr.log`.
- Claude's own session log: the JSONL under `$CLAUDE_CONFIG_DIR/projects/`
  named by the `session_id` in that attempt's `stdout.log`.

## Tests

```sh
make test     # unit + e2e: a real daemon per test, fake workers, temp memento stores
make race     # the same under the race detector
make smoke    # real Claude and Codex workers (uses tokens)
make bench    # memento write latency
```

Tests never touch `~/.kami` or your memento config: each one gets its own
`KAMI_HOME` and a `git init`-ed store through `MEMENTO_STORE`.
