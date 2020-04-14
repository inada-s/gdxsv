#!/bin/bash
# Copy four flycast and Launch game

set -eux

cd $(dirname "$0")

#readonly GDX_ROM_PATH=${GDX_ROM_PATH:-'C:\rom\gdx-disc2\gdx-disc2.gdi'}

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

trap 'kill $(jobs -p)' EXIT
for i in 1 2 3 4; do
  cd work/flycast${i} && MSYS_NO_PATHCONV=1 ./flycast.exe &
done

rm -f work/flycast1/flycast.log
tail -F work/flycast1/flycast.log &
wait $(jobs -l %1 | awk '{print $2}')
