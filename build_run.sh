#!/bin/bash
# Build and Run gdxsv for local development

set -eux

cd $(dirname "$0")

make

export GDXSV_LOBBY_PUBLIC_ADDR="192.168.0.10:9876"
export GDXSV_LOBBY_ADDR="localhost:9876"
export GDXSV_BATTLE_PUBLIC_ADDR="192.168.0.10:9877"
export GDXSV_BATTLE_ADDR="localhost:9877"

./bin/gdxsv -dump -v 3 app 2>&1 | tee log.txt

