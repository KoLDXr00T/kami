workflow(name = "child", inputs = {"n": "int"}, start = "judge")

def pick(v):
    return "bad" if v["n"] == 2 else "good"

state("judge", choose(pick))
state("good", done("good {n}"))
state("bad", fail("bad {n}"))
