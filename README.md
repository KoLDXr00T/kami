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

## Tests

```sh
make test     # unit + e2e: a real daemon per test, fake workers, temp memento stores
make race     # the same under the race detector
make smoke    # real Claude and Codex workers (uses tokens)
make bench    # memento write latency
```

Tests never touch `~/.kami` or your memento config: each one gets its own
`KAMI_HOME` and a `git init`-ed store through `MEMENTO_STORE`.
