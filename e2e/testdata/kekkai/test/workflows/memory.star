workflow(name = "memory", inputs = {"kami_conv": "string?", "kami_peer": "string?", "kami_channel": "string?"}, start = "talk")

def route(v):
    return "listen" if v.get("kami_channel") == "SIG" else "finished"

state("talk", agent("rememberer", "Remember and recall."), done = "route")
state("route", choose(route))
state("finished", done("{heard}| promoted={promoted}"))

# a signal cannot promote a waiting run either
state("listen", wait(signal = "promote", timeout = "1m"), signal = "after", timeout = "after")
state("after", done("{kami_conv}"))
