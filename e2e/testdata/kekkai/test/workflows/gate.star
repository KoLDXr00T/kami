workflow(name = "gate", start = "ship_it")

state("ship_it", choice("Ship it?", body = "Everything is green."), ship = "shipped", hold = "held")
state("shipped", done("shipped"))
state("held", done("held"))
