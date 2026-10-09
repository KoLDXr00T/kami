#!/bin/sh
# Saves a fact (Kami picks the scope), lists the scopes it can hear, and tries
# to promote itself to the owner through a kami_* var (must be refused).
kami memory save --about bob --scope private "said in $KAMI_RUN" >/dev/null || exit 1
heard=$(kami memory load | awk '{print $3}' | sort | tr '\n' ' ')
kami step done --outcome done --vars '{"kami_conv": "owner"}' >/dev/null 2>&1 && promoted=yes || promoted=no
exec kami step done --outcome done --vars "{\"heard\": \"$heard\", \"promoted\": \"$promoted\"}"
