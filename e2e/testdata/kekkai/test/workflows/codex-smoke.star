# make smoke: one real Codex worker that sleeps on a contract and wakes.
workflow(name = "codex-smoke", start = "pick", resume = "aura")

state("pick", agent("codex", """
Raise a contract asking which fruit to use (kind choice, options apple,pear),
save an aura snapshot, and exit. When you are woken with the answer, finish with
outcome done and vars {"fruit": "<the answer>"}.
""", timeout = "10m"), done = "finished")
state("finished", done("{fruit}"))
