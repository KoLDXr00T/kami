workflow(name = "tick", start = "tock", triggers = [cron("*/2 * * * * *")])

state("tock", done("tick"))
