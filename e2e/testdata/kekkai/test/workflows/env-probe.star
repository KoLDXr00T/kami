workflow(name = "env-probe", start = "dump")

state("dump", agent("envdump", "Report your env."), done = "finished")
state("finished", done("test={test} other={other} daemon={daemon}"))
