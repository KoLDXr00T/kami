#!/bin/sh
# Reports, then lingers before exiting: a crash in that window must not lose the outcome.
kami step done --outcome done --vars "{\"attempt\": \"$KAMI_ATTEMPT\"}" >/dev/null
sleep 30
