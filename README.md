gdxsv
---

Mobile Suit Gundam: Federation vs. Zeon & DX private game server.

## Introduction
[Mobile Suit Gundam: Federation vs. Zeon](https://en.wikipedia.org/wiki/Mobile_Suit_Gundam:_Federation_vs._Zeon) was released on Playstation2 and Dreamcast.
This game has online mode, but the service ended in 2004. This project aims to keep the online mode alive for fans.


## Running Server links
- gdxsv : https://www.gdxsv.net
- dreamcastlive : https://dreamcastlive.net/mobile-suit-gundam-federation-vs-zeon-dx/

---

## Build and Run


### Requirements to build
- Go 1.9 or newer with CGO
- stringer. `go get golang.org/x/tools/cmd/stringer`
-	protoc-gen-go v1.25.0 (if modify .proto files)
-	protoc v3.6.1 (if modify .proto files)


### Build and Run gdxsv
1. Clone this repository
1. Run `make` then `bin/gdxsv` is generated. (cgo required)
1. Run `./bin/gdxsv initdb`, then gdxsv.db is generated.
1. Edit build_run.sh to fix server address. (set you PC's address. Don't edit the port number.)
1. Run `./bin/gdxsv lbs`. This command serves one lobby and one matching server.


### LBS and MCS architecture
There are two ways to run the gdxsv binary.
1. `lbs` starts the lobby and the match server one by one.
2. `mcs` starts only the match server and connects to the parent` lbs`.

This allows one lobby server to manage match servers around the world.
There is a CloudFunction script in the `mcsfunc` directory that launches mcs on GCP.

Using only the `lbs` command to act as a standalone lobby and match server. (This is especially useful during local development.)


### Configulations
All settings are specified from environment variables.
- `GDXSV_LOBBY_PUBLIC_ADDR` : Specifies the TCP address that used when a mcs connects to a lbs.
- `GDXSV_LOBBY_ADDR` :  Specifies the TCP address that the lbs listens on. Currently only the port number is used.
- `GDXSV_BATTLE_PUBLIC_ADDR` : Specifies the TCP/UDP address that a client will use to connect with TCP/UDP.
- `GDXSV_BATTLE_ADDR` : Specifies the TCP/UDP address that the mcs listens on. Currently only the port number is used.
- `GDXSV_MCSFUNC_KEY` : Specify a GCP key that has permission to call mcsfunc.
- `GDXSV_MCSFUNC_URL` : Specify an URL of mcsfunc that you deployed.


## Directory structures

### `gdxsv`
The `gdxsv` directory contains main server program.

### `flycast`
The `flycast` directory is a submodule, that is flycast fork customized for the development of this server.

It contains some dirty code for gdxsv, but I would like to deliver the artifacts upstream.

You can download the latest version of flycast built for gdxsv from the [Release page](https://github.com/inada-s/flycast/release) of this repository.

### `pcsx2`
The `pcsx2` directory is a submodule, that is pcsx2 fork customized for the development of this server.
I don't expect to play with this because of performance issues.

The PS2 version of this game has a lot of debug symbols left and useful for analysis.

### `ps2patch`
The `ps2patch` directory contains source program of paches for playstation2 version to enter network mode.
These patches now depends on pcsx2 fork.
The c code will be compiled with [ps2dev-docker](https://github.com/ps2dev/ps2dev-docker) and be applied as cheating codes.

---

# LICENSE
All server-side codes are AGPL-3.0 Licensed.
Submodules and dependent packages are subject to their respective licenses.



