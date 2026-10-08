workflow(name = "signal-wait", inputs = {"key": "string"}, start = "hold")

state("hold", wait(signal = "go", key = "{key}", timeout = "6s"), signal = "got", timeout = "late")
state("got", done("got {payload}"))
state("late", done("late"))
