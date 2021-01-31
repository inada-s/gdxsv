#!/bin/bash

set -eux

cd $(dirname "$0")

export GDXSV_DISCORD_TOKEN=""
export GDXSV_DB_NAME="gdxsv.db"
export GDXSV_SERVICE_KEY="/etc/google/auth/application_default_credentials.json"
export GDXSV_SPREADSHEET_ID="1z7cmpEryrF1hZlF0RqIBxKrTqjm868Og4Nvy5PMa650"

if [[ ! -d $LATEST_TAG/bin ]]; then
  python3 -m venv venv
fi

source venv/bin/activate
python3 -m pip install -r chatops/requirements.txt
exec python3 chatops/bot.py

