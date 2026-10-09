# A conversation loop: each message re-enters "think", which resumes its session.
workflow(name = "chat-loop", inputs = {"thread": "string"}, start = "think", resume = "session", max_visits = 10)

state("think", agent("chatter", "Answer {text}."), replied = "listen")
state("listen", wait(signal = "msg", key = "{thread}", timeout = "4s"), signal = "think", timeout = "closed")
state("closed", done("{resumed_from}"))
