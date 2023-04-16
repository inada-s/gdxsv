package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"gdxsv/gdxsv/proto"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"google.golang.org/api/option"
	pb "google.golang.org/protobuf/proto"

	"cloud.google.com/go/profiler"
	"github.com/caarlos0/env"
	"github.com/jmoiron/sqlx"
	stackdriver "github.com/tommy351/zap-stackdriver"
	"go.uber.org/zap"
)

var (
	// This will be overwritten via ldflags.
	gdxsvVersion  string
	gdxsvRevision string

	// Minimum required flycast version.
	requiredFlycastVersion = "v1.0.5"

	// Global random
	gRand = rand.New(rand.NewSource(time.Now().UnixNano()))
)

var (
	conf Config

	cpu       = flag.Int("cpu", 2, "setting GOMAXPROCS")
	pprof     = flag.Int("pprof", 1, "0: disable pprof, 1: enable http pprof, 2: enable blocking profile")
	cprof     = flag.Int("cprof", 0, "0: disable cloud profiler, 1: enable cloud profiler, 2: also enable mtx profile")
	prodlog   = flag.Bool("prodlog", false, "use production logging mode")
	loglevel  = flag.Int("v", 2, "logging level. 1:error, 2:info, 3:debug")
	mcsdelay  = flag.Duration("mcsdelay", 0, "mcs room delay for network lag emulation")
	noban     = flag.Bool("noban", false, "disable user ban")
	notempban = flag.Bool("notempban", false, "disable temporary ban")
)

var (
	logger *zap.Logger
)

type Config struct {
	LobbyAddr        string `env:"GDXSV_LOBBY_ADDR" envDefault:"localhost:3333"`
	LobbyPublicAddr  string `env:"GDXSV_LOBBY_PUBLIC_ADDR" envDefault:"127.0.0.1:3333"`
	LobbyHttpAddr    string `env:"GDXSV_LOBBY_HTTP_ADDR" envDefault:":3380"`
	BattleAddr       string `env:"GDXSV_BATTLE_ADDR" envDefault:"localhost:3334"`
	BattlePublicAddr string `env:"GDXSV_BATTLE_PUBLIC_ADDR" envDefault:"127.0.0.1:3334"`
	BattleRegion     string `env:"GDXSV_BATTLE_REGION" envDefault:""`
	BattleLogPath    string `env:"GDXSV_BATTLE_LOG_PATH" envDefault:"./battlelog"`

	GCPProjectID string `env:"GDXSV_GCP_PROJECT_ID" envDefault:""`
	GCPKeyPath   string `env:"GDXSV_GCP_KEY_PATH" envDefault:""`
	McsFuncKey   string `env:"GDXSV_MCSFUNC_KEY" envDefault:""` // deprecated
	McsFuncURL   string `env:"GDXSV_MCSFUNC_URL" envDefault:""`

	DBName string `env:"GDXSV_DB_NAME" envDefault:"gdxsv.db"`
}

func printHeader() {
	fmt.Println("   ========================================================================")
	fmt.Println("    gdxsv - Mobile Suit Gundam: Federation vs. Zeon&DX Private Game Server.")
	fmt.Printf("    Version: %v (%v)\n", gdxsvVersion, gdxsvRevision)
	fmt.Println("   ========================================================================")
}

func printUsage() {
	fmt.Print(`
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

`)
	flag.PrintDefaults()
}

func loadConfig() {
	var c Config
	if err := env.Parse(&c); err != nil {
		logger.Fatal("config load failed", zap.Error(err))
	}

	if c.GCPKeyPath == "" && c.McsFuncKey != "" {
		c.GCPKeyPath = c.McsFuncKey
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

	// http pprof
	if 1 <= *pprof {
		if 2 <= *pprof {
			runtime.MemProfileRate = 1
			runtime.SetBlockProfileRate(1)
			runtime.SetMutexProfileFraction(1)
			logger.Warn("mem profile mode enabled")
		}
		go func() {
			port := pprofPort(command)
			addr := fmt.Sprintf(":%v", port)
			err := http.ListenAndServe(addr, nil)
			logger.Error("http.ListenAndServe error", zap.Error(err), zap.String("addr", addr))
		}()
	}

	// google cloud profiler
	if 1 <= *cprof {
		cfg := profiler.Config{
			Service:        fmt.Sprintf("gdxsv-%s", command),
			ServiceVersion: gdxsvVersion,
			ProjectID:      conf.GCPProjectID,
		}
		if 2 <= *cprof {
			cfg.MutexProfiling = true
		}
		if err := profiler.Start(cfg, option.WithCredentialsFile(conf.GCPKeyPath)); err != nil {
			logger.Error("failed to start cloud profiler", zap.Error(err), zap.Any("cfg", cfg))
		}
		logger.Info("profiler started")
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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()

	lbs := NewLbs()
	if *noban {
		lbs.NoBan()
	}
	if *notempban {
		lbs.NoTempBan()
	}
	go lbs.ListenAndServe(stripHost(conf.LobbyAddr))

	mcs := NewMcs(*mcsdelay)
	go mcs.ListenAndServe(stripHost(conf.BattleAddr))
	if conf.LobbyHttpAddr != "" {
		lbs.RegisterHTTPHandlers()
		go func() {
			err := http.ListenAndServe(conf.LobbyHttpAddr, nil)
			if err != nil {
				logger.Error("http.ListenAndServe", zap.Error(err))
			}
		}()
	}

	logger.Sugar()

	<-ctx.Done()
	stop()
	logger.Info("Shutdown")
	lbs.Quit()
	mcs.Quit()
	time.Sleep(100 * time.Millisecond) // Grace to send Shutdown packet
	logger.Info("Bye")
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
	case 0, 1:
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
	defer func() {
		_ = logger.Sync()
	}()

	logger.Info("hello gdxsv",
		zap.String("gdxsv_version", gdxsvVersion),
		zap.String("gdxsv_revision", gdxsvRevision))

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
		_ = os.Remove(conf.DBName)
		prepareDB()
		err := getDB().Init()
		if err != nil {
			logger.Error("InitDB failed", zap.Error(err))
		}
	case "migratedb":
		prepareDB()
		err := getDB().Migrate()
		if err != nil {
			logger.Error("Migration failed:", zap.Error(err))
		} else {
			logger.Info("Migration done")
		}
	case "battlelog2json":
		b, err := os.ReadFile(args[1])
		if err != nil {
			logger.Fatal("Failed to open log file", zap.Error(err))
		}
		logfile := new(proto.BattleLogFile)
		err = pb.Unmarshal(b, logfile)
		if err != nil {
			logger.Fatal("Failed to Unmarshal", zap.Error(err))
		}
		for _, data := range logfile.BattleData {
			fmt.Println(data.UserId, data.Seq, hex.EncodeToString(data.Body))
		}

		logfile.BattleData = nil
		js, err := json.MarshalIndent(logfile, "", "  ")
		if err != nil {
			logger.Fatal("Failed to Unmarshal", zap.Error(err))
		}
		fmt.Print(string(js))

	default:
		printUsage()
		os.Exit(1)
	}
}
