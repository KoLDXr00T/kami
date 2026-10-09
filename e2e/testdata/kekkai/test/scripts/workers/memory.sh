#!/bin/sh
# Saves a fact (Kami picks the scope), lists the scopes it can hear, and tries
# to promote itself to the owner through a kami_* var (must be refused).
kami memory save --about bob --scope private "said in $KAMI_RUN" >/dev/null 2>&1 || saved=no
heard=$(kami memory load 2>/dev/null | awk '{print $3}' | sort | tr '\n' ' ')
[ "$saved" = no ] && heard="none "
kami step done --outcome done --vars '{"kami_conv": "owner"}' >/dev/null 2>&1 && promoted=yes || promoted=no
exec kami step done --outcome done --vars "{\"heard\": \"$heard\", \"promoted\": \"$promoted\"}"
