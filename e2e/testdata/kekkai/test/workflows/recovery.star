workflow(name = "recovery", start = "hold",
         afk = afk("grace", grace = "3s", resolver = "resolvers/approve.sh"))

state("hold", wait(timer = "2s"), timer = "approve")
state("approve", choice("Approve after the pause?"), approve = "ok", reject = "no")
state("ok", done("ok"))
state("no", done("no"))
