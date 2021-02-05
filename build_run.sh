#!/bin/bash
# Build and Run gdxsv for local development

set -eux

cd $(dirname "$0")

make

export GDXSV_LOBBY_PUBLIC_ADDR="192.168.1.10:9876"
export GDXSV_LOBBY_ADDR="localhost:9876"
export GDXSV_BATTLE_PUBLIC_ADDR="192.168.1.10:9877"
export GDXSV_BATTLE_ADDR="localhost:9877"

export GDXSV_GCP_PROJECT_ID=""
export GDXSV_GCP_KEY_PATH=""
export GDXSV_MCSFUNC_URL=""

./bin/gdxsv -v 3 -noban lbs 2>&1 | tee log.txt

