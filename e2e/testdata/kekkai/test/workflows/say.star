# Milestones: say= posts a line when a step finishes, per outcome or for all.
workflow(name = "say", start = "ship_it")

state("ship_it", choice("Ship it?", say = {"ship": "shipping {note}", "hold": "held"}),
      ship = "work", hold = "finished")
state("work", agent("echo", "Work.", say = "worked: {work_out}"), done = "finished")
state("finished", done())
