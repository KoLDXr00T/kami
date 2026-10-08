# One generated clip through the muz analyzer (2–5 minutes, stem separation).

workflow(
    name = "analyze-clip",
    inputs = {"clip_id": "string", "audio": "string"},
    start = "analyze",
)

# Wraps `make analyze FILE=...`; prints {"outcome": "ok", "vars": {"analysis_md": ...}}.
state("analyze", script("scripts/analyze-clip {audio}", cwd = "~/music", timeout = "20m"),
      ok = "analyzed", error = "failed")

state("analyzed", done("{analysis_md}"))
state("failed", fail("analysis failed for {clip_id}"))
