package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/caarlos0/env"
	"github.com/davecgh/go-spew/spew"
	"github.com/jmoiron/sqlx"
	"github.com/tommy351/zap-stackdriver"
	"go.uber.org/zap"
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
	prodlog  = flag.Bool("prodlog", false, "use production logging mode")
	loglevel = flag.Int("v", 2, "logging level. 1:error, 2:info, 3:debug")
	mcsdelay = flag.Duration("mcsdelay", 0, "mcs room delay for network lag emulation")
)

var (
	logger *zap.Logger
	sugger *zap.SugaredLogger
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
	fmt.Println("   ========================================================================")
	fmt.Println("    gdxsv - Mobile Suit Gundam: Federation vs. Zeon&DX Private Game Server.")
	fmt.Printf("    Version: %v (%v)\n", gdxsvVersion, gdxsvRevision)
	fmt.Println("   ========================================================================")
}

func printUsage() {
	fmt.Print(`
Usage: gdxsv [lbs, mcs, initdb]

  lbs: Serve lobby server and default battle server.
    A lbs hosts PS2, DC1 and DC2 version, but their lobbies are separated internally.

  mcs: Serve battle server.
    The mcs attempts to register itself with a lbs.
    When the mcs is vacant for a certain period, it will automatically end.
    It is supposed to host mcs in a different location than the lobby server.

  initdb: Initialize database.
    It is supposed to run this command when first booting manually.
    Note that if the database file already exists it will be permanently deleted.
`)
}

func loadConfig() {
	var c Config
	if err := env.Parse(&c); err != nil {
		logger.Fatal("config load failed", zap.Error(err))
	}

	logger.Info("config loaded", zap.Any("config", c))
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
			err := http.ListenAndServe(addr, nil)
			logger.Error("http.ListenAndServe error", zap.Error(err), zap.String("addr", addr))
		}()
	}
	if *profile >= 2 {
		runtime.MemProfileRate = 1
		runtime.SetBlockProfileRate(1)
		logger.Warn("mem profile mode enabled")
	}
}

var defaultdb DB

func getDB() DB {
	return defaultdb
}

func prepareDB() {
	conn, err := sqlx.Open("sqlite3", conf.DBName)
	if err != nil {
		logger.Fatal("failed to open database", zap.Error(err))
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

	logger.Sugar()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	if *dump {
		dumper := spew.NewDefaultConfig()
		dumper.MaxDepth = 7
		dumper.SortKeys = true
		dumper.DisableMethods = true
		dumper.DisablePointerMethods = true
		dumper.DisablePointerAddresses = true
		go func() {
			for {
				ioutil.WriteFile("dump.txt", []byte(dumper.Sdump(lbs.userPeers)), 0644)
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
		logger.Error("failed to dial lbs", zap.Error(err))
	}
}

func prepareLogger() {
	var err error
	var zapConfig zap.Config

	if *prodlog {
		zapConfig = zap.NewProductionConfig()
		zapConfig.EncoderConfig = stackdriver.EncoderConfig
	} else {
		zapConfig = zap.NewDevelopmentConfig()
		zapConfig.Encoding = "console"
	}

	switch *loglevel {
	case 0,1:
		zapConfig.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	case 2:
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	case 3:
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	default:
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	logger, err = zapConfig.Build()
	if err != nil {
		log.Fatalf("Failed to setup logger: %v", err)
	}

	if *prodlog {
		logger = logger.With(
			zap.String("gdxsv_version", gdxsvVersion),
			zap.String("gdxsv_revision", gdxsvRevision))
	}
}

func main() {
	printHeader()
	flag.Parse()

	prepareLogger()
	defer logger.Sync()

	logger.Info("hello gdxsv",
		zap.String("gdxsv_version", gdxsvVersion),
		zap.String("gdxsv_revision", gdxsvRevision))

	rand.Seed(time.Now().UnixNano())
	args := flag.Args()

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
