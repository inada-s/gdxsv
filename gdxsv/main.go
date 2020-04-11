package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/caarlos0/env"
	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"github.com/jmoiron/sqlx"
	"github.com/miekg/dns"
)

var (
	conf    Config
	dump    = flag.Bool("dump", false, "enable var dump to dump.txt")
	cpu     = flag.Int("cpu", 2, "setting GOMAXPROCS")
	profile = flag.Int("profile", 1, "0: no profile, 1: enable http pprof, 2: enable blocking profile")
)

type Config struct {
	LobbyAddr        string `env:"GDXSV_LOBBY_ADDR" envDefault:"localhost:3333"`
	LobbyPublicAddr  string `env:"GDXSV_LOBBY_PUBLIC_ADDR" envDefault:"127.0.0.1:3333"`
	BattleAddr       string `env:"GDXSV_BATTLE_ADDR" envDefault:"localhost:3334"`
	BattlePublicAddr string `env:"GDXSV_BATTLE_PUBLIC_ADDR" envDefault:"127.0.0.1:3334"`
	DBName           string `env:"GDXSV_DB_NAME" envDefault:"gdxsv.db"`
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

func printUsage() {
	glog.Info("Usage: ", os.Args[0], "[lobby, dns, initdb]")
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

func mainApp() {
	app := NewApp()
	go app.ListenAndServeLobby(stripHost(conf.LobbyAddr))
	go app.ListenAndServeBattle(stripHost(conf.BattleAddr))

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	if *dump {
		dumper := spew.NewDefaultConfig()
		dumper.MaxDepth = 5
		dumper.SortKeys = true
		dumper.DisableMethods = true
		dumper.DisablePointerMethods = true
		dumper.DisablePointerAddresses = true
		go func() {
			for {
				ioutil.WriteFile("dump.txt", []byte(dumper.Sdump(app.users)), 0644)
				time.Sleep(time.Second)
			}
		}()
	}
	s := <-c
	fmt.Println("Got signal:", s)
	app.Quit()
}

func makeDNSHandler(record string) func(dns.ResponseWriter, *dns.Msg) {
	return func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		rr, err := dns.NewRR(record)
		if err != nil {
			glog.Error(err)
		}
		m.Answer = append(m.Answer, rr)
		err = w.WriteMsg(m)
		if err != nil {
			glog.Error(err)
		}
	}
}

func mainDNS() {
	ip, _, err := net.SplitHostPort(conf.LobbyPublicAddr)
	if err != nil {
		glog.Errorln(err)
	}

	dns.HandleFunc("ca1203.mmcp6", makeDNSHandler("ca1203.mmcp6. 3600 IN A "+ip))
	dns.HandleFunc("ca1202.mmcp6", makeDNSHandler("ca1202.mmcp6. 3600 IN A "+ip))

	server := &dns.Server{Addr: ":53", Net: "udp"}
	err = server.ListenAndServe()
	glog.Error(err)
}

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	glog.Infoln("========================================================================")
	glog.Infoln(" gdxsv - Mobile Suit Gundam: Federation vs. Zeon&DX Private Game Server.")
	glog.Infoln("========================================================================")

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
	case "dns":
		mainDNS()
	case "app":
		prepareDB()
		mainApp()
	case "initdb":
		os.Remove(conf.DBName)
		prepareDB()
		getDB().Init()
	default:
		printUsage()
		os.Exit(1)
	}
}
