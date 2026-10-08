workflow(name = "linear", inputs = {"topic": "string"}, start = "first")

state("first", agent("echo", "First pass on {topic}."), done = "second")
state("second", agent("echo", "Second pass."), done = "finished")
state("finished", done("finished {topic} {first_out} {second_out}"))
