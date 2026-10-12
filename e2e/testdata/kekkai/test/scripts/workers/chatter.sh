#!/bin/sh
# Reports which session it was asked to resume, then prints a session id the
# way claude's JSON output does, so the next turn can resume it.
kami step done --outcome replied --vars "{\"resumed_from\": \"${KAMI_RESUME_SESSION:-none}\"}" > /dev/null || exit 1
echo '{"session_id": "sess-1"}'
