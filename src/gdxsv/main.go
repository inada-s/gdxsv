package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"gdxsv/pkg/config"

	"github.com/golang/glog"
)

var cpu = flag.Int("cpu", 2, "setting GOMAXPROCS")
var profile = flag.Int("profile", 1, "0: no profile, 1: enable http pprof, 2: enable blocking profile")

func pprofPort(mode string) int {
	switch mode {
	case "lobby":
		return 16061
	case "battle":
		return 16062
	case "dns":
		return 16063
	case "login":
		return 16064
	case "status":
		return 16065
	default:
		return 16060
	}
}

func stripHost(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		glog.FatalDepth(1, "err in splitPort ", addr, err)
	}
	return ":" + fmt.Sprint(port)
}

func printUsage() {
	log.Println("Usage: ", os.Args[0], "[lobby]")
}

func prepareOption(command string) {
	runtime.GOMAXPROCS(*cpu)
	if *profile >= 1 {
		go func() {
			port := pprofPort(command)
			addr := fmt.Sprintf(":%v", port)
			log.Println(http.ListenAndServe(addr, nil))
		}()
	}
	if *profile >= 2 {
		runtime.MemProfileRate = 1
		runtime.SetBlockProfileRate(1)
	}
}

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	glog.Infoln("gdxsv - GundamDX private game server.")

	args := flag.Args()
	glog.Infoln(args, len(args))

	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}

	config.LoadConfig()

	command := args[0]
	prepareOption(command)

	switch command {
	case "lobby":
		// prepareDB()
		mainLobby()
	default:
		printUsage()
		os.Exit(1)
	}
}
