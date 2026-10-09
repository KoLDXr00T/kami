workflow(name = "memory", inputs = {"kami_conv": "string?", "kami_peer": "string?", "kami_channel": "string?"}, start = "talk")

state("talk", agent("rememberer", "Remember and recall."), done = "finished")
state("finished", done("{heard}| promoted={promoted}"))
