#!/bin/bash

set -eux

cd $(dirname "$0")

readonly LATEST_TAG=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.tag_name')
readonly DOWNLOAD_URL=$(curl -sL https://api.github.com/repos/inada-s/gdxsv/releases/latest | jq -r '.assets[].browser_download_url')

if [[ ! -d $LATEST_TAG/bin ]]; then
  echo "Downloading latest version..."
  mkdir -p "$LATEST_TAG"
  pushd "$LATEST_TAG"
    wget "$DOWNLOAD_URL"
    tar xzvf bin.tgz && rm bin.tgz
  popd
fi

GDXSV_BIN=$LATEST_TAG/bin/gdxsv

export GDXSV_LOBBY_PUBLIC_ADDR="153.121.44.150:9876"
export GDXSV_LOBBY_ADDR=":9876"
export GDXSV_LOBBY_HTTP_ADDR=":9880"
export GDXSV_BATTLE_PUBLIC_ADDR="153.121.44.150:9877"
export GDXSV_BATTLE_ADDR=":9877"
export GDXSV_DB_NAME="gdxsv.db"
export GDXSV_GCP_PROJECT_ID="gdxsv-274515"
export GDXSV_GCP_KEY_PATH="/etc/google/auth/application_default_credentials.json"
export GDXSV_MCSFUNC_URL="https://asia-northeast1-gdxsv-274515.cloudfunctions.net/mcsfunc"

exec "$GDXSV_BIN" -prodlog -cprof=2 lbs >> /var/log/gdxsv-lbs.log 2>&1
