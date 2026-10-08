#!/bin/sh
# Takes a while, so a test can kill the daemon mid-attempt.
if grep -q "daemon restarted" "$KAMI_PROMPT_FILE"; then
  exec kami step done --outcome done --vars '{"recovered": "yes"}'
fi
sleep 30
exec kami step done --outcome done --vars '{"recovered": "no"}'
