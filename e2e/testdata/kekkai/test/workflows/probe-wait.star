workflow(name = "probe-wait", start = "poll")

state("poll", wait(probe = "scripts/probe.sh", every = "500ms", timeout = "20s"),
      ready = "finished", timeout = "late")
state("finished", done("{ready_at}"))
state("late", fail("probe never became ready"))
