package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/caarlos0/env"
	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"github.com/jmoiron/sqlx"
)

var (
	// This will be overwritten via ldflags.
	gdxsvVersion  string
	gdxsvRevision string
)

var (
	conf     Config
	dump     = flag.Bool("dump", false, "enable var dump to dump.txt")
	cpu      = flag.Int("cpu", 2, "setting GOMAXPROCS")
	profile  = flag.Int("profile", 1, "0: no profile, 1: enable http pprof, 2: enable blocking profile")
	mcsdelay = flag.Duration("mcsdelay", 0, "mcs room delay for network lag emulation")
)

type Config struct {
	LobbyAddr        string `env:"GDXSV_LOBBY_ADDR" envDefault:"localhost:3333"`
	LobbyPublicAddr  string `env:"GDXSV_LOBBY_PUBLIC_ADDR" envDefault:"127.0.0.1:3333"`
	BattleAddr       string `env:"GDXSV_BATTLE_ADDR" envDefault:"localhost:3334"`
	BattlePublicAddr string `env:"GDXSV_BATTLE_PUBLIC_ADDR" envDefault:"127.0.0.1:3334"`
	BattleRegion     string `env:"GDXSV_BATTLE_REGION" envDefault:""`
	McsFuncKey       string `env:"GDXSV_MCSFUNC_KEY" envDefault:""`
	McsFuncURL       string `env:"GDXSV_MCSFUNC_URL" envDefault:""`
	DBName           string `env:"GDXSV_DB_NAME" envDefault:"gdxsv.db"`
}

func printHeader() {
	glog.Infoln("========================================================================")
	glog.Infoln(" gdxsv - Mobile Suit Gundam: Federation vs. Zeon&DX Private Game Server.")
	glog.Infof(" Version: %v (%v)\n", gdxsvVersion, gdxsvRevision)
	glog.Infoln("========================================================================")
}

func printUsage() {
	glog.Info("")
	glog.Info("Usage: gdxsv [lbs, mcs, initdb]")
	glog.Info("")
	glog.Info("	lbs: Serve lobby server and default battle server.")
	glog.Info("	  A lbs hosts PS2, DC1 and DC2 version, but their lobbies are separated internally.")
	glog.Info("")
	glog.Info("	mcs: Serve battle server.")
	glog.Info("	  The mcs attempts to register itself with a lbs.")
	glog.Info("	  When the mcs is vacant for a certain period, it will automatically end.")
	glog.Info("	  It is supposed to host mcs in a different location than the lobby server.")
	glog.Info("")
	glog.Info("	initdb: Initialize database.")
	glog.Info("	  It is supposed to run this command when first booting manually.")
	glog.Info("	  Note that if the database file already exists it will be permanently deleted.")
}

func loadConfig() {
	var c Config
	if err := env.Parse(&c); err != nil {
		glog.Fatal(err)
	}

	glog.Infof("%+v", c)
	conf = c
}

func pprofPort(mode string) int {
	switch mode {
	case "lbs":
		return 26061
	case "mcs":
		return 26062
	default:
		return 26060
	}
}

func prepareOption(command string) {
	runtime.GOMAXPROCS(*cpu)
	if *profile >= 1 {
		go func() {
			port := pprofPort(command)
			addr := fmt.Sprintf(":%v", port)
			glog.Errorln(http.ListenAndServe(addr, nil))
		}()
	}
	if *profile >= 2 {
		runtime.MemProfileRate = 1
		runtime.SetBlockProfileRate(1)
	}
}

var defaultdb DB

func getDB() DB {
	return defaultdb
}

func prepareDB() {
	conn, err := sqlx.Open("sqlite3", conf.DBName)
	if err != nil {
		glog.Fatal(err)
	}

	defaultdb = SQLiteDB{
		DB:          conn,
		SQLiteCache: NewSQLiteCache(),
	}
}

func mainLbs() {
	lbs := NewLbs()
	go lbs.ListenAndServe(stripHost(conf.LobbyAddr))
	defer lbs.Quit()

	mcs := NewMcs(*mcsdelay)
	go mcs.ListenAndServe(stripHost(conf.BattleAddr))
	defer mcs.Quit()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	if *dump {
		dumper := spew.NewDefaultConfig()
		dumper.MaxDepth = 7
		dumper.SortKeys = true
		dumper.DisableMethods = true
		dumper.DisablePointerMethods = true
		dumper.DisablePointerAddresses = true
		go func() {
			for {
				ioutil.WriteFile("dump.txt", []byte(dumper.Sdump(lbs.users)), 0644)
				time.Sleep(time.Second)
			}
		}()
	}
	s := <-c
	fmt.Println("Got signal:", s)
}

func mainMcs() {
	mcs := NewMcs(*mcsdelay)
	go mcs.ListenAndServe(stripHost(conf.BattleAddr))
	defer mcs.Quit()

	err := mcs.DialAndSyncWithLbs(conf.LobbyPublicAddr, conf.BattlePublicAddr, conf.BattleRegion)
	if err != nil {
		glog.Error(err)
	}
}

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	printHeader()

	args := flag.Args()
	glog.Infoln(args, len(args))

	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}

	loadConfig()

	command := args[0]
	prepareOption(command)

	switch command {
	case "lbs":
		prepareDB()
		mainLbs()
	case "mcs":
		mainMcs()
	case "initdb":
		os.Remove(conf.DBName)
		prepareDB()
		getDB().Init()
	default:
		printUsage()
		os.Exit(1)
	}
}
