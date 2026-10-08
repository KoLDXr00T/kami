# Daily: import songs made on Suno outside a `song` run.

workflow(
    name = "suno-sync",
    inputs = {},
    start = "download",
    triggers = [cron("0 9 * * *")],
)

state("download", script("scripts/suno-download", timeout = "30m"),
      ok = "imported", pending = "imported", login = "relogin")

state("relogin", blocker("The Suno session expired in the capture Chrome profile"),
      done = "download", skip = "imported")

state("imported", done("{imported} new clips imported"))
