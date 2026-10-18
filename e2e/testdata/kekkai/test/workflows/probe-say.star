# A pending probe's say is posted only when it changes.
workflow(name = "probe-say", start = "poll")

state("poll", wait(probe = "scripts/probe-say.sh", every = "200ms", timeout = "20s"),
      ready = "finished", timeout = "late")
state("finished", done())
state("late", fail("probe never became ready"))
