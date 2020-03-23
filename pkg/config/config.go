package config

import (
	"log"

	"github.com/caarlos0/env/v6"
	"github.com/golang/glog"
)

// Conf stores global config.
var Conf Config

// LoadConfig loads environmental variable into Conf.
func LoadConfig() {
	var c Config
	if err := env.Parse(&c); err != nil {
		log.Fatal(err)
	}
	glog.Infof("%+v", c)
	Conf = c
}

// Config stores gdxsv config.
type Config struct {
	LobbyAddr        string `env:"GDXSV_LOBBY_ADDR"`
	LobbyPublicAddr  string `env:"GDXSV_LOBBY_PUBLIC_ADDR"`
	BattleAddr       string `env:"GDXSV_BATTLE_ADDR"`
	BattlePublicAddr string `env:"GDXSV_BATTLE_PUBLIC_ADDR"`
	DBName           string `env:"GDXSV_DB_NAME"`
}
