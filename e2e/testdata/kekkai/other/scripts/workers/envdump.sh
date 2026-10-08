#!/bin/sh
exec kami step done --outcome done --vars "{\"test\": \"${SECRET_TEST:-}\", \"other\": \"${SECRET_OTHER:-}\", \"daemon\": \"${DAEMON_SECRET:-}\"}"
