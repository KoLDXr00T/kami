#!/bin/sh
# pending twice, then ready
n=$(cat count 2>/dev/null || echo 0)
n=$((n + 1))
echo "$n" > count
if [ "$n" -lt 3 ]; then
  echo '{"outcome": "pending"}'
else
  echo "{\"outcome\": \"ready\", \"vars\": {\"ready_at\": $n}}"
fi
