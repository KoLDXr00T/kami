workflow(name = "linger", start = "work")

state("work", agent("linger", "Report, then linger."), done = "finished")
state("finished", done("attempt={attempt}"))
