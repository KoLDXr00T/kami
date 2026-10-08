workflow(name = "blocker-wake", start = "paint")

state("paint", agent("sleeper", "Paint the wall."), done = "painted")
state("painted", done("{answer_seen} aura={saw_aura}"))
