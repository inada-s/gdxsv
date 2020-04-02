#!/bin/bash -ex
#ee-addr2line  ee-c++        ee-g++        ee-gcov       ee-objcopy    ee-readelf    ee-strip
#ee-ar         ee-c++filt    ee-gcc        ee-ld         ee-objdump    ee-size
#ee-as         ee-cpp        ee-gccbug     ee-nm         ee-ranlib     ee-strings
#

rm -f $(pwd)/patch/main.o
rm -f $(pwd)/patch/main.x
rm -f $(pwd)/patch/gdxpatch.o

docker run -v $(pwd):$(pwd) \
    ps2dev-docker ee-gcc -O0 \
    $(pwd)/patch/main.c \
    -c -o $(pwd)/patch/main.x

docker run -v $(pwd):$(pwd) \
    ps2dev-docker ee-ld \
    -T $(pwd)/patch/ld.script \
    $(pwd)/patch/main.x \
    -o $(pwd)/patch/main.o

docker run -v $(pwd):$(pwd) \
    ps2dev-docker ee-objdump \
    -h $(pwd)/patch/main.o

docker run -v $(pwd):$(pwd) \
    ps2dev-docker ee-objcopy \
    --only-section gdx.init \
    --only-section gdx.data \
    --only-section gdx.func \
    $(pwd)/patch/main.o \
    $(pwd)/patch/gdxpatch.o

docker run -v $(pwd):$(pwd) \
    ps2dev-docker ee-objdump \
    -D $(pwd)/patch/gdxpatch.o | tee $(pwd)/patch/gdxpatch.asm
