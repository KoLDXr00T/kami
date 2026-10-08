# One Suno song: concept → prompt file → you generate → download → analyze → you rate.

workflow(
    name = "song",
    inputs = {"concept": "string"},
    start = "write",
)

MUZ = "~/music"

def clip_input(clip, v):
    return {"clip_id": clip["id"], "audio": clip["audio"]}

def analysis_report(v):
    return "\n\n".join([c["report"] for c in v["children"]])

def verdict(v):
    return "next_concept" if v["rating"] == "bad" else "finished"

# ── Write the prompt ────────────────────────────────────────────────────

state("write",
      agent("writer", "Write a Suno song for: {concept}", skills = ["suno-prompt"], cwd = MUZ,
            inputs = ["concept", "mood", "analysis"]),
      done = "lint", no_mood = "mood")

# The answer lands in vars["mood"], which `write` receives.
state("mood", ask("Name the mood of '{concept}' in one word"),
      answered = "write")

state("lint", script("make lint", cwd = MUZ, max_visits = 3),
      ok = "generate", error = "write")

# ── Generate (you: Suno has no API) and download ────────────────────────

state("generate", action("Generate {song_file} on Suno, then resolve", body_file = "{song_file}"),
      done = "download", skip = "finished")

state("download", wait(probe = "scripts/suno-download", every = "10m", timeout = "2h"),
      ok = "analyze", login = "relogin", timeout = "generate")

state("relogin", blocker("The Suno session expired in the capture Chrome profile"),
      done = "download")

# ── Analyze each clip, then you rate ────────────────────────────────────

state("analyze", spawn("analyze-clip", for_each = "clips", input = clip_input),
      all_done = "rate", any_failed = "rate")

state("rate",
      form("Rate {title}", body = analysis_report, lock = True,
           fields = {"best_clip": "string", "rating": "enum:bad,good,favorite", "replay": "bool"}),
      rated = "record")

state("record", script("scripts/record-rating"),
      ok = "verdict")

state("verdict", choose(verdict))

# A bad track gets a new concept, never a re-roll of the same brief.
state("next_concept",
      form("{title} underdelivered. New concept, or stop?", fields = {"concept": "string"}),
      new_concept = "write", stop = "finished")

state("finished", done("{song_file}: {rating}"))
