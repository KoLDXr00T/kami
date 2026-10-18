#!/bin/sh
# Reports which session it was asked to resume, then prints a session id the
# way claude's JSON output does (with its cost), so the next turn can resume it.
kami step done --outcome replied --vars "{\"resumed_from\": \"${KAMI_RESUME_SESSION:-none}\"}" > /dev/null || exit 1
echo '{"session_id": "sess-1", "total_cost_usd": 0.25, "duration_ms": 1200, "usage": {"input_tokens": 3, "output_tokens": 7}}'
