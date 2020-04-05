#!/bin/bash
# Build and Run gdxsv for local development

set -eux

cd $(dirname "$0")

make

export GDXSV_LOBBY_PUBLIC_ADDR="localhost:3333"
export GDXSV_LOBBY_ADDR="localhost:3333"

./bin/gdxsv -dump -v 3 lobby 2>&1 | tee log.txt

