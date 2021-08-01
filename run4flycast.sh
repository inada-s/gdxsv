#!/bin/bash
# Copy four flycast and Launch game

set -eux

cd $(dirname "$0")

readonly N=${N:-"4"}
readonly GDXSV=${GDXSV:-"zdxsv.net"}
readonly GDX_ROM_PATH=${GDX_ROM_PATH:-'C:\rom\gdx-disc2\gdx-disc2.gdi'}
#readonly GDX_ROM_PATH=${GDX_ROM_PATH:-'C:\rom\gdx-disc1\gdx-disc1.gdi'}


flycast[1]="flycast/build/artifact/flycast.exe"
flycast[2]="flycast/build/artifact/flycast.exe"
flycast[3]="flycast/build/artifact/flycast.exe"
flycast[4]="flycast/build/artifact/flycast.exe"
#flycast[2]="work/bin/flycast-merge-upstream-20210614.exe"
#flycast[3]="work/bin/Flycast-win_x64-v0.7.8-0f5ef2e.exe"
#flycast[4]="work/bin/Flycast-win_x64-v0.7.8-0f5ef2e.exe"

for i in $(seq "${N}"); do
  mkdir -p work/flycast${i}/data
  if [[ i < ${#flycast[@]} ]] && [[ -f ${flycast[i]} ]]; then
    cp ${flycast[i]} work/flycast${i}/flycast.exe
  else
    echo "Not found: flycast$i use 1"
    cp ${flycast[1]} work/flycast${i}/flycast.exe
  fi
done

for i in $(seq "${N}"); do
  sed -i "s/^server =.*$/server = ${GDXSV}/" work/flycast${i}/emu.cfg
  echo "replacing emu.cfg 'server = ${GDXSV}'"
done

trap 'kill $(jobs -p)' EXIT
for i in $(seq "${N}"); do
  cd work/flycast${i} && MSYS_NO_PATHCONV=1 ./flycast.exe "${GDX_ROM_PATH}" "$@" &
done

rm -f work/flycast1/flycast.log
tail -F work/flycast1/flycast.log &
wait $(jobs -l %1 | awk '{print $2}')
