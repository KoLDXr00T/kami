workflow(name = "timer", start = "nap")

state("nap", wait(timer = "2s"), timer = "awake")
state("awake", done("awake"))
