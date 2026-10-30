workflow(name = "system-lock", start = "ship_it")

state("ship_it", gate("Ship it?"), ship = "shipped", hold = "held")
state("shipped", done("shipped"))
state("held", done("held"))
