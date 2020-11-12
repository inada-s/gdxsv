gdxsv
---

Mobile Suit Gundam: Federation vs. Zeon & DX private game server.

## Introduction
[Mobile Suit Gundam: Federation vs. Zeon](https://en.wikipedia.org/wiki/Mobile_Suit_Gundam:_Federation_vs._Zeon) was released on Playstation2 and Dreamcast.
This game has online mode, but the service ended in 2004. This project aims to keep the online mode alive for fans.


## Running Server links
- gdxsv : https://gdxsv.net
- dreamcastlive : https://dreamcastlive.net/mobile-suit-gundam-federation-vs-zeon-dx/


---

### Directory structures

### `gdxsv`
The `gdxsv` directory contains main server program.

### `pcsx2`
The `pcsx2` directory is a submodule, that is pcsx2 fork customized for the development of this server.
I don't expect to play with this because of performance issues.

### `ps2patch`
The `ps2patch` directory contains source program of paches for playstation2 version to enter network mode.
These patches now depends on pcsx2 fork.
The c code will be compiled with [ps2dev-docker](https://github.com/ps2dev/ps2dev-docker) and be applied as cheating codes.

### `flycast`
The `flycast` directory is a submodule, that is flycast fork customized for the development of this server.

It contains some dirty code for gdxsv, but I would like to deliver the artifacts upstream.

I expect for players to play DC version with flycast. 

### `dcpatch`
The `dcpatch` directory contains patch codes of dreamcast binary.
Eventually it will be export to dreamcast emulator, so it is for work.


