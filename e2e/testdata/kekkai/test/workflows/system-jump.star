workflow(name = "system-jump", start = "poll")

state("poll", wait(probe = "scripts/probe-second.sh", every = "1h", timeout = "1h"),
      ready = "ship_it", timeout = "late")
state("ship_it", gate("Ship it?"), ship = "shipped", hold = "held")
state("shipped", done("shipped"))
state("held", done("held"))
state("late", fail("never woke"))
