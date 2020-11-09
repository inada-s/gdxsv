#!/bin/bash
# Build and Run gdxsv for local development

set -eux

cd $(dirname "$0")

#make race
make

export GDXSV_LOBBY_PUBLIC_ADDR="192.168.1.10:9876"
export GDXSV_LOBBY_ADDR="localhost:9876"
export GDXSV_BATTLE_PUBLIC_ADDR="192.168.1.10:9877"
export GDXSV_BATTLE_ADDR="localhost:9877"
export GDXSV_MCSFUNC_KEY="${HOME}/keys/gdxsv-service-key.json"
export GDXSV_MCSFUNC_URL="https://asia-northeast1-gdxsv-274515.cloudfunctions.net/mcsfunc"
export GDXSV_DISCORD_LIVESTATUS_WEBHOOK_URL="{PASTE_WEBHOOK_URL_HERE}/messages/{PASTE_TARGET_MESSAGE_ID_HERE}"

#./bin/gdxsv -v 3 lbs -profile 1 2>&1 | tee log.txt
./bin/gdxsv -v 3 lbs 2>&1 | tee log.txt

