# make smoke: same as claude-smoke, but the wake resumes the Claude session.
workflow(name = "claude-resume", start = "pick", resume = "session")

state("pick", agent("claude", """
First, remember the secret word "lantern" (do not write it anywhere).
Then raise a contract asking which fruit to use (kind choice, options apple,pear),
and exit without saving an aura. When the contract is answered, finish with outcome
done and vars {"fruit": "<the answer>", "word": "<the secret word>"}.
""", timeout = "10m"), done = "finished")
state("finished", done("{fruit} {word}"))
