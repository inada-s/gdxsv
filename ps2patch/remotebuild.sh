#!/bin/bash 

set -eux

# set docker available host
readonly SSH_HOST=${SSH_HOST:-mbp}
readonly SCRIPT_DIR=$(cd $(dirname $0); pwd)

rm -rf $SCRIPT_DIR/bin
mkdir -p $SCRIPT_DIR/bin

cd $SCRIPT_DIR/..
ssh ${SSH_HOST} rm -rf /tmp/ps2patch
scp -r ps2patch ${SSH_HOST}:/tmp/ps2patch
ssh ${SSH_HOST} /tmp/ps2patch/build.sh
scp -r ${SSH_HOST}:/tmp/ps2patch/bin $SCRIPT_DIR

cd $SCRIPT_DIR
./asm2pnach.py < bin/gdxpatch.asm > $SCRIPT_DIR/../pcsx2/bin/cheats/1187BBDF.pnach
