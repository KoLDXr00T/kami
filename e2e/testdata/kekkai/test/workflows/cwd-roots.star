workflow(name = "cwd-roots", inputs = {"dir": "string"}, start = "where")

state("where", script("scripts/pwd.sh", cwd = "{dir}"), ok = "finished")
state("finished", done("{pwd}"))
