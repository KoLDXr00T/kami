workflow(name = "loop-budget", start = "spin", max_visits = 3)

state("spin", script("true"), ok = "spin")
