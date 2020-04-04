#!/bin/bash
cd $(dirname "$0")
# 00009426 = 37926
#make && GDXSV_LOBBY_PUBLIC_ADDR="localhost:37926" GDXSV_LOBBY_ADDR="localhost:37926" ./bin/gdxsv -dump -v 3 lobby 2>&1 | tee log.txt
make && GDXSV_LOBBY_PUBLIC_ADDR="localhost:3333" GDXSV_LOBBY_ADDR="localhost:3333" ./bin/gdxsv -dump -v 3 lobby 2>&1 | tee log.txt

