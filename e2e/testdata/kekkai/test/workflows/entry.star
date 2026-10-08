workflow(name = "entry", inputs = {"goal": "string?"}, start = "begin",
         entries = {"followup": entry("follow", require = ["goal"])})

state("begin", done("began"))
state("follow", done("followed {goal}"))
