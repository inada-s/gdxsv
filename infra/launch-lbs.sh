#!/bin/bash

set -eux

cd $(dirname $0)

# Download latest version from github release
readonly LATEST_TAG=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.tag_name')
readonly DOWNLOAD_URL=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.assets[].browser_download_url')

if [[ ! -d $LATEST_TAG/bin ]]; then
  echo "Downloading latest version..."
  mkdir -p $LATEST_TAG
  pushd $LATEST_TAG
    wget $DOWNLOAD_URL
    tar xzvf bin.tgz && rm bin.tgz
  popd
fi

PUBLIC_IP=$(curl -s https://ipinfo.io/ip)
export GDXSV_LOBBY_PUBLIC_ADDR="${PUBLIC_IP}:9876"
export GDXSV_LOBBY_ADDR=":9876"
export GDXSV_BATTLE_PUBLIC_ADDR="${PUBLIC_IP}:9877"
export GDXSV_BATTLE_ADDR=":9877"
export GDXSV_DB_NAME="gdxsv.db"

# TODO: remove -v=3
exec $LATEST_TAG/bin/gdxsv lbs -v=3
