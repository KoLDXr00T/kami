#!/bin/sh
# Reports done and records which state it ran in.
exec kami step done --outcome done --vars "{\"${KAMI_STEP}_out\": \"ok\"}"
