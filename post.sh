#!/usr/bin/env bash

set -euo pipefail

source .env

address=${ADDRESS:-'https://www.gthm.is/new'}

if [[ $# -eq 0 ]]; then
        echo Use: $0 TITLE
        echo TITLE is required.
        echo Write post in STDIN. Then CTRL-D to post, CTRL-C to cancel.
fi

curl -X POST \
        --user "$USER:$PASS" \
        --data-urlencode title="$@" \
        --data-urlencode body@- \
        "$address"
