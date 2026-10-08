---
name: kami-worker
description: The protocol every Kami worker (Shikigami) follows. Loaded automatically into every Kami agent step. Covers how to finish a step (`kami step done`), how to stop on a blocker without asking in chat (`kami contract request` + `kami state snapshot`, then exit), and how to resume from an Aura snapshot after waking.
---

# Kami worker protocol

You are running one step of a Kami workflow, headless. Nobody reads your chat
output. The only ways to talk to the workflow are the `kami` commands below.
`KAMI_RUN`, `KAMI_STEP` and `KAMI_TOKEN` are already set in your environment.

## Finish the step

When the instruction is done, run exactly once:

```sh
kami step done --outcome <outcome> [--vars '{"key": "value"}']
```

- `<outcome>` must be one of the outcomes listed in your prompt. Any other value
  is refused with the allowed list; fix it and run the command again.
- `--vars` is a JSON object merged into the run's vars. Later steps read them
  (`{key}` in their instructions), so report what they need: paths of files you
  wrote, ids you created, verdicts. Large text goes in a file; pass the path.
- Report as soon as the outcome is known, before any cleanup or summary. A
  session that dies after the work but before `kami step done` loses the work.
- Exit after reporting.

## Blocked: ask out of band, then sleep

If you hit ambiguity or a blocker you cannot resolve (missing decision,
expired credentials, a choice only a human can make), do not ask in chat and
do not guess. Instead:

1. Raise a contract:

   ```sh
   kami contract request --kind <ask|choice|blocker> --title "<one-line question>" \
     [--body "<context for someone who has not seen your work>"] \
     [--options "a,b,c"] [--default a]
   ```

2. Save an Aura snapshot so the next worker can continue without your context:

   ```sh
   kami state snapshot <<'EOF'
   {"goal": "what this step is trying to achieve",
    "progress": ["what is already done, one line each"],
    "next": "exactly what to do once the contract is answered",
    "vars": {"anything the next worker needs": "paths, ids"}}
   EOF
   ```

3. Exit. The run sleeps until someone (or an AFK resolver) answers. A fresh
   worker then starts with your Aura and the answer. It does not see this
   conversation.

## On wake-up

If your prompt has a "Waking up" section, a previous worker slept on a
contract. Trust its Aura: do not redo anything listed under `progress`,
start from `next`, and use the resolution it was waiting for.

## Before touching the world

A previous attempt may have crashed half-way. Before creating, posting,
pushing or filing anything, check whether it already happened.
