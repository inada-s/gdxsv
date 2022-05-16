[![LICENSE](https://img.shields.io/github/license/inada-s/gdxsv)](LICENSE)
[![GitHub go.mod Go version of a Go module](https://img.shields.io/github/go-mod/go-version/inada-s/gdxsv)](https://github.com/inada-s/gdxsv)
[![GoReportCard example](https://goreportcard.com/badge/github.com/inada-s/gdxsv)](https://goreportcard.com/report/github.com/inada-s/gdxsv)
[![codecov](https://codecov.io/gh/inada-s/gdxsv/branch/master/graph/badge.svg?token=WD6DL2ZT5G)](https://codecov.io/gh/inada-s/gdxsv)



Mobile Suit Gundam: Federation vs. Zeon & DX private game server.

The brother project [zdxsv is here](https://github.com/inada-s/zdxsv).

## Introduction
[Mobile Suit Gundam: Federation vs. Zeon](https://en.wikipedia.org/wiki/Mobile_Suit_Gundam:_Federation_vs._Zeon) was released on Playstation2 and Dreamcast.
This game has online mode, but the service ended in 2004. This project aims to keep the online mode alive for fans.

## Running Server links
- https://www.gdxsv.net : This is the server hosted by this project. Good for flycast emulator players.
- https://dreamcastlive.net/mobile-suit-gundam-federation-vs-zeon-dx/ : Dreamcastlive's community server. Good for real-dreamcast players.


## Contributors ✨

<!-- ALL-CONTRIBUTORS-BADGE:START - Do not remove or modify this section -->
[![All Contributors](https://img.shields.io/badge/all_contributors-7-orange.svg?style=flat-square)](#contributors-)
<!-- ALL-CONTRIBUTORS-BADGE:END -->

Thanks goes to these wonderful people ([emoji key](https://allcontributors.org/docs/en/emoji-key)):

<!-- ALL-CONTRIBUTORS-LIST:START - Do not remove or modify this section -->
<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
<table>
  <tr>
    <td align="center"><a href="https://github.com/inada-s"><img src="https://avatars.githubusercontent.com/u/1726079?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Shingo INADA</b></sub></a><br /><a href="https://github.com/inada-s/gdxsv/commits?author=inada-s" title="Code">💻</a></td>
    <td align="center"><a href="https://github.com/vkedwardli"><img src="https://avatars.githubusercontent.com/u/602245?v=4?s=100" width="100px;" alt=""/><br /><sub><b>vkedwardli</b></sub></a><br /><a href="https://github.com/inada-s/gdxsv/commits?author=vkedwardli" title="Code">💻</a> <a href="#financial-vkedwardli" title="Financial">💵</a></td>
    <td align="center"><a href="https://github.com/htc001120"><img src="https://avatars.githubusercontent.com/u/6858053?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Ming Chan</b></sub></a><br /><a href="https://github.com/inada-s/gdxsv/commits?author=htc001120" title="Code">💻</a> <a href="#financial-htc001120" title="Financial">💵</a></td>
    <td align="center"><a href="https://github.com/Q-SJO-Q"><img src="https://avatars.githubusercontent.com/u/86608532?v=4?s=100" width="100px;" alt=""/><br /><sub><b>Q-SJO-Q</b></sub></a><br /><a href="#financial-Q-SJO-Q" title="Financial">💵</a></td>
    <td align="center"><a href="https://github.com/crazytaka3"><img src="https://avatars.githubusercontent.com/u/86925395?v=4?s=100" width="100px;" alt=""/><br /><sub><b>crazytaka3</b></sub></a><br /><a href="#financial-crazytaka3" title="Financial">💵</a></td>
    <td align="center"><a href="https://www.facebook.com/Mobile.Suit.Gundam.DX/"><img src="https://avatars.githubusercontent.com/u/87101475?v=4?s=100" width="100px;" alt=""/><br /><sub><b>HK-DX-Players</b></sub></a><br /><a href="#financial-HK-DX-Players" title="Financial">💵</a></td>
    <td align="center"><a href="https://github.com/SMGMpartner"><img src="https://avatars.githubusercontent.com/u/102720932?v=4?s=100" width="100px;" alt=""/><br /><sub><b>SMGMpartner</b></sub></a><br /><a href="#financial-SMGMpartner" title="Financial">💵</a></td>
  </tr>
</table>

<!-- markdownlint-restore -->
<!-- prettier-ignore-end -->

<!-- ALL-CONTRIBUTORS-LIST:END -->

This project follows the [all-contributors](https://github.com/all-contributors/all-contributors) specification. Contributions of any kind welcome!

---

## Build and Run


### Requirements to build
- Go 1.16 or newer with CGO
- stringer. (Run `make install-tools` to install)
- protoc-gen-go (Run `make install-tools` to install)
- protoc v3.6.1 (if modify .proto files)


### Build and Run gdxsv
1. Clone this repository
1. Run `make` then `bin/gdxsv` is generated. (cgo required)
1. Run `./bin/gdxsv initdb`, then gdxsv.db is generated.
1. Edit build_run.sh to fix server address. (set you PC's address. Don't edit the port number.)
1. Run `./bin/gdxsv lbs`. This command serves one lobby and one matching server.


### LBS and MCS architecture
There are two ways to run the gdxsv binary.
1. `lbs` serves a lobby and a match server one by one.
2. `mcs` serves only a match server and connects to the parent `lbs`.

This allows one lobby server to manage match servers around the world.
There is a CloudFunction script in the `mcsfunc` directory that launches mcs on GCP.

Using only the `lbs` command to act as a standalone lobby and match server. (This is especially useful during local development.)


### Configulations

#### Environment variables
- `GDXSV_LOBBY_PUBLIC_ADDR` : Specifies the TCP address that used when a mcs connects to a lbs.
- `GDXSV_LOBBY_ADDR` :  Specifies the TCP address that the lbs listens on. Currently only the port number is used.
- `GDXSV_BATTLE_PUBLIC_ADDR` : Specifies the TCP/UDP address that a client will use to connect with TCP/UDP.
- `GDXSV_BATTLE_ADDR` : Specifies the TCP/UDP address that the mcs listens on. Currently only the port number is used.
- `GDXSV_BATTLE_LOG_PATH` : Specifies a file path that will be used to save battle log file.
- `GDXSV_GCP_PROJECT_ID` : Specifies the project id of Google Cloud Platform. Required if you use mcsfunc or CloudProfiler.
- `GDXSV_GCP_KEY_PATH` : Specifies a GCP Service Account keyfile that have permission for following roles.
  - `roles/cloudfunctions.invoker`
  - `roles/cloudprofiler.agent`
- `GDXSV_MCSFUNC_URL` : Specifies a URL of mcsfunc that you deployed.

#### Commandline arguments
```
Usage: gdxsv <Flags...> [lbs, mcs, initdb, migratedb]

  lbs: Serve lobby server and default battle server.
    A lbs hosts PS2, DC1 and DC2 version, but their lobbies are separated internally.

  mcs: Serve battle server.
    The mcs attempts to register itself with a lbs.
    When the mcs is vacant for a certain period, it will automatically end.
    It is supposed to host mcs in a different location than the lobby server.

  initdb: Initialize database.
    It is supposed to run this command before you run lbs first time.
    Note that if the database file already exists it will be permanently deleted.

  migratedb: Update database schema.
    It is supposed to run this command before you run updated gdxsv.

  battlelog2json: Convert battle log file to json.

Flags:

  -cprof int
        0: disable cloud profiler, 1: enable cloud profiler, 2: also enable mtx profile
  -cpu int
        setting GOMAXPROCS (default 2)
  -mcsdelay duration
        mcs room delay for network lag emulation
  -noban
        not to check bad users
  -pprof int
        0: disable pprof, 1: enable http pprof, 2: enable blocking profile (default 1)
  -prodlog
        use production logging mode
  -v int
        logging level. 1:error, 2:info, 3:debug (default 2)
```

## Directory structures

### `gdxsv`
The `gdxsv` directory contains main server program.

### `flycast`
The `flycast` directory is a submodule, that is flycast fork customized for the development of this server.

It contains some dirty code for gdxsv, but I would like to deliver the artifacts upstream someday....

You can download the latest version of flycast built for gdxsv from the [Release page](https://github.com/inada-s/flycast/release) of this repository.

### `pcsx2`
The `pcsx2` directory is a submodule, that is pcsx2 fork customized for the development of this server.
I don't expect to play with this for now, but PS2 version has a lot of debug symbols left and useful for analysis.

---

# LICENSE
All server-side codes are AGPL-3.0 Licensed.
Submodules and dependent packages are subject to their respective licenses.


