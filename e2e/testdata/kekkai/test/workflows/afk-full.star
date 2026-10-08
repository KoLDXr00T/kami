workflow(name = "afk-full", start = "approve", afk = afk("afk", resolver = "resolvers/approve.sh"))

state("approve", choice("Approve the draft?"), approve = "approved", reject = "rejected")
state("approved", done("approved"))
state("rejected", done("rejected"))
