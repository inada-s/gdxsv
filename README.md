gdxsv
---
# UNDER CONSTRUCTION


## DEVELOPOR INFORMATION
### gdxsv
The `gdxsv` directory contains main server program.

### pcsx2 fork
The `pcsx2` directory is a submodule, that is pcsx2 fork customized for the development of this server.
I don't expect to play with this because of performance issues.

### ps2patch
The `ps2patch` directory contains source program of paches for playstation2 version to enter network mode.
These patches now depends on pcsx2 fork.


### Development

Recommended developer environments.
#### Windows
- Visual Studio Community 2019 (for pcsx2)
- Visual Studio Code
- Go 1.9 for windows
- Git bash
- make on Git bash https://sourceforge.net/projects/ezwinports/files/make-4.3-without-guile-w32-bin.zip/download
- tdm64-gcc-9.2.0.exe https://jmeubank.github.io/tdm-gcc/

run `make` on git bash to build gdxsv.
