workflow(name = "silent", start = "mute")

state("mute", agent("silent", "Do nothing useful."), done = "finished")
state("finished", done())
