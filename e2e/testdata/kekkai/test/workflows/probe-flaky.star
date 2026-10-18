# The probe fails {errors} times before it is ready. strict blocks after 2 failures in a row.
workflow(name = "probe-flaky", start = "poll", inputs = {"errors": "int", "strict": "bool"})

state("poll", choose(lambda v: "strict" if v["strict"] else "lenient"))
state("lenient", wait(probe = "scripts/flaky.sh {errors}", every = "100ms", timeout = "20s"),
      ready = "finished", timeout = "late")
state("strict", wait(probe = "scripts/flaky.sh {errors}", every = "100ms", timeout = "20s", max_errors = 2),
      ready = "finished", timeout = "late")
state("finished", done("ready"))
state("late", fail("probe never became ready"))
