#!/bin/bash
cd $(dirname "$0")
make && GDXSV_LOBBY_PUBLIC_ADDR="localhost:3333" GDXSV_LOBBY_ADDR="localhost:3333" ./bin/gdxsv -v 3 lobby 
