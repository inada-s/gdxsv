#!/bin/bash
# Copy four pcsx2/bin and Launch game

set -eux

cd $(dirname "$0")

readonly GDX_ISO_PATH=${GDX_ISO_PATH-'C:\rom\GDX.ISO'}

for i in 1 2 3 4; do
  if [[ ! -d "pcsx2/bin${i}" ]]; then
    cp -r pcsx2/bin pcsx2/bin${i}
  fi
  cp pcsx2/bin/pcsx2-dev.exe pcsx2/bin${i}/pcsx2-dev.exe
  #cp -r pcsx2/bin/cheats/* pcsx2/bin${i}/cheats
  rm -rf pcsx2/bin${i}/cheats/*
  cp -r pcsx2/bin/inis pcsx2/bin${i}/inis
done

echo "bin12367" > pcsx2/bin1/gdxsv_loginkey.txt
echo "bin24567" > pcsx2/bin2/gdxsv_loginkey.txt
echo "bin34567" > pcsx2/bin3/gdxsv_loginkey.txt
echo "bin44567" > pcsx2/bin4/gdxsv_loginkey.txt

sed -i -e 's#MainGuiPosition.*$#MainGuiPosition=0,0#'      pcsx2/bin1/inis/PCSX2_ui.ini
sed -i -e 's#MainGuiPosition.*$#MainGuiPosition=800,0#'    pcsx2/bin2/inis/PCSX2_ui.ini
sed -i -e 's#MainGuiPosition.*$#MainGuiPosition=0,550#'    pcsx2/bin3/inis/PCSX2_ui.ini
sed -i -e 's#MainGuiPosition.*$#MainGuiPosition=800,550#'  pcsx2/bin4/inis/PCSX2_ui.ini

sed -i -e 's#DisplayPosition.*$#DisplayPosition=0,0#'      pcsx2/bin1/inis/PCSX2_ui.ini
sed -i -e 's#DisplayPosition.*$#DisplayPosition=800,0#'    pcsx2/bin2/inis/PCSX2_ui.ini
sed -i -e 's#DisplayPosition.*$#DisplayPosition=0,550#'    pcsx2/bin3/inis/PCSX2_ui.ini
sed -i -e 's#DisplayPosition.*$#DisplayPosition=800,550#'  pcsx2/bin4/inis/PCSX2_ui.ini

sed -i -e 's#WindowPos.*$#WindowPos=0,0#'      pcsx2/bin1/inis/PCSX2_ui.ini
sed -i -e 's#WindowPos.*$#WindowPos=800,0#'    pcsx2/bin2/inis/PCSX2_ui.ini
sed -i -e 's#WindowPos.*$#WindowPos=0,550#'    pcsx2/bin3/inis/PCSX2_ui.ini
sed -i -e 's#WindowPos.*$#WindowPos=800,550#'  pcsx2/bin4/inis/PCSX2_ui.ini

# edit stick binding for bin3, bin4 
for bin in bin3 bin4; do
sed -i -e '/, 32, /d' pcsx2/${bin}/inis/LilyPad.ini
sed -i -e '/, 33, /d' pcsx2/${bin}/inis/LilyPad.ini
sed -i -e '/, 34, /d' pcsx2/${bin}/inis/LilyPad.ini
sed -i -e '/, 35, /d' pcsx2/${bin}/inis/LilyPad.ini
sed -i -e 's#, 36, #, 32, #' pcsx2/${bin}/inis/LilyPad.ini
sed -i -e 's#, 37, #, 33, #' pcsx2/${bin}/inis/LilyPad.ini
sed -i -e 's#, 38, #, 34, #' pcsx2/${bin}/inis/LilyPad.ini
sed -i -e 's#, 39, #, 35, #' pcsx2/${bin}/inis/LilyPad.ini
done

trap 'kill $(jobs -p)' EXIT
MSYS_NO_PATHCONV=1 pcsx2/bin1/pcsx2-dev.exe ${GDX_ISO_PATH} &
MSYS_NO_PATHCONV=1 pcsx2/bin2/pcsx2-dev.exe ${GDX_ISO_PATH} &
MSYS_NO_PATHCONV=1 pcsx2/bin3/pcsx2-dev.exe ${GDX_ISO_PATH} &
MSYS_NO_PATHCONV=1 pcsx2/bin4/pcsx2-dev.exe ${GDX_ISO_PATH} &
wait $(jobs -l %1 | awk '{print $2}')
