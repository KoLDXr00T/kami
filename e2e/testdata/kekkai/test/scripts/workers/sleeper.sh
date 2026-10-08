#!/bin/sh
# First attempt: raise a contract, save an aura, exit (sleep).
# On wake: read the answer and the aura from the prompt, then finish.
if grep -q "## Waking up" "$KAMI_PROMPT_FILE"; then
  answer=$(grep -o '"answer": "[a-z]*"' "$KAMI_PROMPT_FILE" | head -1 | sed 's/.*: "\(.*\)"/\1/')
  aura=no
  grep -q '"primed the wall"' "$KAMI_PROMPT_FILE" && aura=yes
  exec kami step done --outcome done --vars "{\"answer_seen\": \"$answer\", \"saw_aura\": \"$aura\"}"
fi
kami contract request --kind choice --title "Which color?" --options red,blue --body "The wall is primed." || exit 1
kami state snapshot <<'AURA' || exit 1
{"goal": "paint the wall", "progress": ["primed the wall"], "next": "paint it in the chosen color"}
AURA
exit 0
