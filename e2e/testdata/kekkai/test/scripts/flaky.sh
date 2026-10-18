#!/bin/sh
# fails (exit 1, no JSON) the first $1 times, then ready
n=$(cat count 2>/dev/null || echo 0)
n=$((n + 1))
echo "$n" > count
if [ "$n" -le "$1" ]; then
  echo "upstream 503" >&2
  exit 1
fi
echo '{"outcome": "ready"}'
