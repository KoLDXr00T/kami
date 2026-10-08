workflow(name = "spawn", inputs = {"items": "list"}, start = "fan")

def child_input(item, v):
    return {"n": item}

state("fan", spawn("child", for_each = "items", input = child_input),
      all_done = "all_good", any_failed = "some_failed")
state("all_good", done("all good"))
state("some_failed", done("some failed"))
