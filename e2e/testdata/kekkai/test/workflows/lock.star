# The workflow is fully AFK, but a gate is still only yours.
workflow(name = "lock", start = "merge_gate", afk = afk("afk", resolver = "resolvers/approve.sh"))

state("merge_gate", gate("Merge it?"), approve = "merged", hold = "held")
state("merged", done("merged"))
state("held", done("held"))
