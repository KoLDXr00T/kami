# make smoke: one real Claude worker that sleeps on a contract and wakes.
workflow(name = "claude-smoke", start = "pick", resume = "aura")

state("pick", agent("claude", """
Raise a contract asking which fruit to use (kind choice, options apple,pear),
save an aura snapshot, and exit. When you are woken with the answer, finish with
outcome done and vars {"fruit": "<the answer>"}.
""", timeout = "10m"), done = "finished")
state("finished", done("{fruit}"))
