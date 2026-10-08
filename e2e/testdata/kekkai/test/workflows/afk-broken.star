workflow(name = "afk-broken", start = "approve", afk = afk("afk", resolver = "resolvers/broken.sh"))

state("approve", choice("Approve the draft?"), approve = "approved", reject = "rejected")
state("approved", done("approved"))
state("rejected", done("rejected"))
