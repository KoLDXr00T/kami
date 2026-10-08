workflow(name = "script-step", inputs = {"word": "string"}, start = "emit")

state("emit", script("scripts/emit.sh {word}"), said = "fail_on_purpose", error = "broken")
state("fail_on_purpose", script("sh -c 'exit 3'"), ok = "unexpected", error = "broken")
state("broken", done("{echo}|{stdin_has_word}"))
state("unexpected", fail("exit 3 routed as ok"))
