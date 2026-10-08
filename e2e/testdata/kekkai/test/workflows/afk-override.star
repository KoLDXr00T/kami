# attended by default; the resolver is there for live overrides
workflow(name = "afk-override", start = "approve", afk = afk("attended", resolver = "resolvers/approve.sh"))

state("approve", choice("Approve the draft?"), approve = "approved", reject = "rejected")
state("approved", done("approved"))
state("rejected", done("rejected"))
