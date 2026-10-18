# A heartbeat posts its script's stdout while the step runs; a failing beat
# is logged and the step carries on.
workflow(name = "heartbeat", inputs = {"word": "string"}, start = "hold")

state("hold", wait(timer = "2s", heartbeat = heartbeat("scripts/beat.sh {word}", every = "300ms")),
      timer = "finished")
state("finished", done())
