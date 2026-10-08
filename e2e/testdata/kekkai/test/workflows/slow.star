workflow(name = "slow", start = "grind")

state("grind", agent("slow", "Grind for a while."), done = "finished")
state("finished", done("recovered={recovered}"))
