#!/bin/sh
# Reports whether the prompt carries only the allowlisted var.
saw=visible-only
grep -q "SEEN-VALUE" "$KAMI_PROMPT_FILE" || saw=missing
grep -q "HIDDEN-VALUE" "$KAMI_PROMPT_FILE" && saw=leak
exec kami step done --outcome done --vars "{\"saw\": \"$saw\"}"
