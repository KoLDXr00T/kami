workflow(name = "inputs-scope", inputs = {"visible": "string", "secret": "string"}, start = "look")

state("look", agent("inputs", "Look around.", inputs = ["visible"]), done = "finished")
state("finished", done("{saw}"))
