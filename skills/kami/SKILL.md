---
name: kami
description: Drive the Kami agent OS from a Claude Code session — start workflow runs, see and resolve the decisions (contracts) waiting on the user, set AFK modes, publish signals, read a run's history, switch the active Kekkai domain. Use when the user says "kami", "start a kami run", "what's waiting on me", "resolve that contract", "afk this run", "kami status", or names a Kami workflow or Kekkai.
---

# kami — operate the daemon

Kami runs Starlark workflows (`~/.kami/kekkai/<domain>/workflows/*.star`)
whose steps are headless Claude/Codex workers or scripts. Decisions park as
**contracts** in a queue instead of in chat. Durable state lives in the
memento namespace `kami`; the event log in `~/.kami/events.db`.

Always pass `--json` when you parse output. Ids accept any unique prefix.

## Is it up?

```sh
kami status                 # daemon, run counts, open contracts, cron triggers
kami daemon start           # if it is not running (launchd normally keeps it up)
```

Exit code 6 means the daemon is unreachable.

## Domains (Kekkai)

```sh
kami kekkai list            # * marks the active one
kami kekkai switch <name>   # CLI default only; every domain keeps running
kami kekkai validate        # config, skills, workers, every workflow
kami kekkai reload          # after editing cron triggers
```

## Runs

```sh
kami workflow list                                   # in the active kekkai
kami run start <workflow> --input k=v [--input ...]  # values parse as JSON when they can
kami run start <workflow>:<entry> --input goal=x     # a named entry point
kami run start <workflow> --afk grace=10m            # AFK for this run
kami run list --status active                        # running, sleeping, waiting
kami run show <id>                                   # state, vars, contracts, aura, events
kami run cancel <id>                                 # cancels children too
```

A run's status: `running` (worker/script executing), `sleeping` (a worker
raised a contract and exited), `waiting` (decision, wait or child runs),
`done`, `failed`, `cancelled`. Terminal runs report `outcome` (the terminal
state's name) and `report`.

## Decisions waiting on the user

```sh
kami contract list                     # open ones; --status all for history
kami contract show <id>                # title, body, answers, fields, mode
kami contract resolve <id> <answer>    # answer = one of the listed answers
kami contract resolve <id> "free text" # an ask: any text
kami contract resolve <id> rated --field rating=good --field replay=true   # a form
```

Rules when acting for the user:

- **Never resolve a contract the user has not answered.** Show it (title,
  body verbatim, the answers) and ask. A contract marked `you only` (a
  `gate`, an `action`, or `lock=True`) exists precisely so nothing but the
  user decides it.
- Relay the body verbatim. It was written for someone who has not seen the work.
- `CONFLICT ... already resolved by resolver` means the AFK resolver got there
  first; report that instead of retrying.

## AFK

Each contract runs `attended` (only the user), `grace=<d>` (the user has d,
then the resolver answers) or `afk` (resolver at once). The workflow sets a
default; overrides win in this order: contract, run, workflow, kekkai default.

```sh
kami afk show <run>                          # effective mode per open contract
kami afk set afk --run <id>                  # this run's open + future contracts
kami afk set grace=30m --contract <id>       # one contract
kami afk set attended --run <id>             # back to the user
```

Locked contracts refuse `afk set` (exit 5). A resolver that fails hands the
contract back to `attended`; it is never dropped.

## Signals and history

```sh
kami signal pub <type> --key <correlation key> --data '{"k": "v"}'
kami events tail -f [--type contract.] [--run <id>]   # NDJSON with --json
```

`kami events tail -f --json` is the stream future adapters (jane, phoenix,
Slack) consume.

## When something is stuck

A run that cannot continue parks on an engine **blocker** contract
(answers `retry` / `stop`, or `continue` / `stop` for a loop budget). Its body
carries the error and the step's log directory; read
`<dir>/stderr.log` and `<dir>/prompt.md` there before retrying.
