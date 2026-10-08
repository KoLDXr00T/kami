workflow(name = "afk-grace", start = "approve",
         afk = afk("grace", grace = "2s", resolver = "resolvers/approve.sh"))

state("approve", choice("Approve the draft?"), approve = "approved", reject = "rejected")
state("approved", done("approved"))
state("rejected", done("rejected"))
