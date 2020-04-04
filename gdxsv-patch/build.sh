#!/bin/bash -ex
#ee-addr2line  ee-c++        ee-g++        ee-gcov       ee-objcopy    ee-readelf    ee-strip
#ee-ar         ee-c++filt    ee-gcc        ee-ld         ee-objdump    ee-size
#ee-as         ee-cpp        ee-gccbug     ee-nm         ee-ranlib     ee-strings

SCRIPT_DIR=$(cd $(dirname $0); pwd)
cd $SCRIPT_DIR

PATH=$PATH:/usr/local/bin

docker run -v $(pwd):$(pwd) \
    ps2dev-docker ee-gcc -O0 -G0 \
    $(pwd)/src/main.c \
    -c -o $(pwd)/bin/main.x

docker run -v $(pwd):$(pwd) \
    ps2dev-docker ee-ld \
    -T $(pwd)/src/ld.script \
    $(pwd)/bin/main.x \
    -o $(pwd)/bin/main.o

docker run -v $(pwd):$(pwd) \
    ps2dev-docker ee-objdump \
    -h $(pwd)/bin/main.o

docker run -v $(pwd):$(pwd) \
    ps2dev-docker ee-objcopy \
    --only-section gdx.inject \
    --only-section gdx.main \
    --only-section gdx.data \
    --only-section gdx.func \
    $(pwd)/bin/main.o \
    $(pwd)/bin/gdxpatch.o

docker run -v $(pwd):$(pwd) \
    ps2dev-docker ee-objdump \
    -D $(pwd)/bin/gdxpatch.o > $(pwd)/bin/gdxpatch.asm

