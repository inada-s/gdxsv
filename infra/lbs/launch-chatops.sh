#!/bin/bash

set -eux

cd $(dirname "$0")

if [[ ! -d $LATEST_TAG/bin ]]; then
  python3 -m venv venv
fi

source venv/bin/activate
python3 -m pip install -r chatops/requirements.txt
exec python3 chatops/bot.py

