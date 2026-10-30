#!/bin/sh
# pending once, then ready
if [ -f seen ]; then
  echo '{"outcome": "ready"}'
else
  touch seen
  echo '{"outcome": "pending"}'
fi
