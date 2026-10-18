#!/bin/sh
# beat 1 prints, beat 2 fails, later beats print again
n=$(cat count 2>/dev/null || echo 0)
n=$((n + 1))
echo "$n" > count
[ "$n" -eq 2 ] && { echo "status down" >&2; exit 1; }
echo "  beat $1 $n  "
