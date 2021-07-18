#!/bin/bash
# Build and Run gdxsv for local development

set -eux

cd $(dirname "$0")

make

readonly LOCALIP=${LOCALIP:-"192.168.1.34"}

export GDXSV_LOBBY_PUBLIC_ADDR="${LOCALIP}:9876"
export GDXSV_LOBBY_ADDR="localhost:9876"
export GDXSV_BATTLE_PUBLIC_ADDR="${LOCALIP}:9877"
export GDXSV_BATTLE_ADDR="localhost:9877"

export GDXSV_GCP_PROJECT_ID=""
export GDXSV_GCP_KEY_PATH=""
export GDXSV_MCSFUNC_URL=""

exec ./bin/gdxsv -v 3 -noban lbs 2>&1
