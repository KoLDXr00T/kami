#!/bin/sh
# says "building" twice, then "testing", then ready
n=$(cat count 2>/dev/null || echo 0)
n=$((n + 1))
echo "$n" > count
case $n in
  1|2) echo '{"outcome": "pending", "say": "building"}' ;;
  3) echo '{"outcome": "pending", "say": "testing"}' ;;
  *) echo '{"outcome": "ready"}' ;;
esac
