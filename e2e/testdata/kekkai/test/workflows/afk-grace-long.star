# A long grace window, for the case where a human answers first.
workflow(name = "afk-grace-long", start = "approve",
         afk = afk("grace", grace = "30s", resolver = "resolvers/approve.sh"))

state("approve", choice("Approve the draft?"), approve = "approved", reject = "rejected")
state("approved", done("approved"))
state("rejected", done("rejected"))
