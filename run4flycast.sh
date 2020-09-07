#!/bin/bash
# Copy four flycast and Launch game

set -eux

cd $(dirname "$0")

readonly GDXSV=${GDXSV:-"zdxsv.net"}
readonly MAXLAG=${MAXLAG:-"8"}
readonly GDX_ROM_PATH=${GDX_ROM_PATH:-'C:\rom\gdx-disc2\gdx-disc2.gdi'}

mkdir -p work/bin

ls ./flycast/shell/linux/artifacts || true
ls ~/Downloads/flycast.zip || true

if [[ -f ./flycast/shell/linux/artifacts/flycast.exe ]]; then
  set +x
  echo "======================="
  echo "USE local build version"
  echo "======================="
  sleep 2
  set -x
  cp ./flycast/shell/linux/artifacts/flycast.exe work/bin/flycast.exe
elif [[ -f ~/Downloads/flycast.zip  ]]; then
  set +x
  echo "======================="
  echo "USE ci build version"
  echo "======================="
  sleep 2
  set -x
  ## TODO download
  mv ~/Downloads/flycast.zip ./work/bin/
  pushd work/bin
    unzip flycast.zip
    mv ./flycast.*exe flycast.exe || true
  popd
fi

for i in 1 2 3 4; do
  mkdir -p work/flycast${i}/data
  cp work/bin/flycast.exe work/flycast${i}/
done

for i in 1 2 3 4; do
  sed -i "s/^server =.*$/server = ${GDXSV}/" work/flycast${i}/emu.cfg
  sed -i "s/^maxlag =.*$/maxlag = ${MAXLAG}/" work/flycast${i}/emu.cfg
  echo "replacing emu.cfg 'server = ${GDXSV}'"
  echo "replacing emu.cfg 'maxlag= ${MAXLAG}'"
done

trap 'kill $(jobs -p)' EXIT
for i in 1 2 3 4; do
  cd work/flycast${i} && MSYS_NO_PATHCONV=1 ./flycast.exe ${GDX_ROM_PATH} &
done

rm -f work/flycast1/flycast.log
tail -F work/flycast1/flycast.log &
wait $(jobs -l %1 | awk '{print $2}')
