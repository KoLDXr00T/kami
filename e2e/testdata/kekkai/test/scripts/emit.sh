#!/bin/sh
# Echoes its argument and whether the vars arrived on stdin.
stdin=$(cat)
has=no
case "$stdin" in *'"word"'*) has=yes;; esac
printf '{"outcome": "said", "vars": {"echo": "%s", "stdin_has_word": "%s"}}\n' "$1" "$has"
