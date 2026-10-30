workflow(name = "system-wait", start = "poll")

# pending until a second probe, and the next one is an hour away: only a system instruction gets there
state("poll", wait(probe = "scripts/probe-second.sh", every = "1h", timeout = "1h"),
      ready = "finished", moved = "other", timeout = "late")
state("other", done("moved {note}"))
state("finished", done("woke"))
state("late", fail("never woke"))
