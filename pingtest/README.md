## Running precompiled ping.elf

Add the DNS entry into your `emu.cfg`
```
[network]
DNS = 192.168.0.2
```
The ping.elf will then use `192.168.0.2` as the Ping Server IP address.

Then run the server:
`go run pingserver.go`

and the Flycast with the elf binary:
Windows: `C:\flycast.exe "Full\Path\To\ping.elf"`
macOS: `/Applications/Flycast.app/Contents/MacOS/Flycast Full/Path/To/ping.elf`

p.s. remember to set 

## Compiling ping.c

After installing & compiling KallistiOS
`source /opt/toolchains/dc/kos/environ.sh`
`make`


## Remarks
```
//Enable the serial console to view stdout
[config]
Debug.SerialConsoleEnabled = yes 

//Customize what to log
[log]
AICA = no
AICA_ARM = no
AUDIO = no
BOOT = yes
COMMON = no
DYNAREC = no
FLASHROM = no
GDROM = no
HOLLY = no
INPUT = no
INTERPRETER = no
JVS = no
LogToFile = no
MAPLE = no
MEMORY = no
MODEM = yes
NAOMI = no
PVR = no
REIOS = no
RENDERER = no
SAVESTATE = yes
SH4 = no
VMEM = no
Verbosity = 5
```
