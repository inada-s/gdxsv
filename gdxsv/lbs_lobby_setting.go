package main

import (
	"fmt"
)

const (
	PingLimitTh = 64
)

var (
	lbsLobbySettings map[uint16]*LobbySetting
)

// PS2 LobbyID: 1-23
// DC2 LobbyID: 2, 4-6, 9-17, 19-22

type LobbySetting struct {
	Name        string
	McsRegion   string
	Comment     string
	Description string // automatically filled

	AutoReBattle        int
	FreeRule            bool // Allow all Stage and MS/MA
	EnableForceStartCmd bool
	TeamShuffle         bool
	PingLimit           bool
	No375MS             bool
	Cost630             bool
	UnlimitedAmmo       bool

	BeamMSEvent    bool
	LowCostMSEvent bool
}

func init() {
	lbsLobbySettings = map[uint16]*LobbySetting{
		// Earth lobbies
		1: {
			// PS2 Only
			Name:                "",
			McsRegion:           "",
			EnableForceStartCmd: true,
			Comment:             "1",
		},
		2: {
			Name:                "タクラマカン砂漠",
			McsRegion:           "",
			EnableForceStartCmd: true,
			TeamShuffle:         true,
			Comment:             "Default Server",
		},
		3: {
			// PS2 Only
			Name:                "",
			McsRegion:           "",
			PingLimit:           true,
			EnableForceStartCmd: true,
			Comment:             "1",
		},
		4: {
			Name:                "黒海南岸森林地帯",
			McsRegion:           "asia-east2",
			PingLimit:           true,
			EnableForceStartCmd: true,
		},
		5: {
			Name:         "オデッサ",
			McsRegion:    "asia-east2",
			TeamShuffle:  true,
			PingLimit:    true,
			Comment:      "TeamShuffle 3R",
			AutoReBattle: 3,
		},
		6: {
			Name:         "ベルファスト",
			McsRegion:    "asia-northeast1",
			TeamShuffle:  true,
			PingLimit:    true,
			Comment:      "TeamShuffle 3R",
			AutoReBattle: 3,
		},
		7: {
			// PS2 Only
			Name:                "",
			McsRegion:           "",
			EnableForceStartCmd: true,
			Comment:             "1",
		},
		8: {
			// PS2 Only
			Name:                "",
			McsRegion:           "",
			EnableForceStartCmd: true,
			Comment:             "1",
		},
		9: {
			Name:                "ニューヤーク",
			McsRegion:           "asia-northeast1",
			PingLimit:           true,
			EnableForceStartCmd: true,
		},
		10: {
			Name:                "グレートキャニオン",
			McsRegion:           "asia-northeast1",
			PingLimit:           true,
			EnableForceStartCmd: true,
			No375MS:             true,
			Comment:             "No 375 Cost MS",
		},
		11: {
			Name:                "ジャブロー",
			McsRegion:           "asia-northeast2",
			PingLimit:           true,
			EnableForceStartCmd: true,
		},
		12: {
			Name:                "地下基地",
			McsRegion:           "asia-east1",
			PingLimit:           true,
			EnableForceStartCmd: true,
			Comment:             "For JP vs HK",
		},

		// Universe lobbies
		13: {
			Name:                "ソロモン",
			McsRegion:           "best",
			EnableForceStartCmd: true,
			FreeRule:            true,
			Comment:             "Free Lobby",
		},
		14: {
			Name:                "ソロモン宙域",
			McsRegion:           "best",
			EnableForceStartCmd: true,
			FreeRule:            true,
			Comment:             "Free Lobby",
		},
		15: {
			Name:           "ア・バオア・クー宙域",
			McsRegion:      "best",
			LowCostMSEvent: true,
			UnlimitedAmmo:  true,
			Comment:        "Event Lobby",
		},
		16: {
			Name:           "ア・バオア・クー外部",
			McsRegion:      "best",
			LowCostMSEvent: true,
			UnlimitedAmmo:  true,
			Comment:        "Event Lobby",
		},
		17: {
			Name:           "ア・バオア・クー内部",
			McsRegion:      "best",
			LowCostMSEvent: true,
			UnlimitedAmmo:  true,
			TeamShuffle:    true,
			Comment:        "Event Lobby (TeamShuffle)",
		},
		18: {
			// PS2 Only
			Name:                "",
			McsRegion:           "best",
			EnableForceStartCmd: true,
			FreeRule:            true,
			Comment:             "Free Lobby",
		},
		19: {
			Name:                "衛星軌道1",
			McsRegion:           "best",
			EnableForceStartCmd: true,
			No375MS:             true,
			Comment:             "No 375 Cost MS",
		},
		20: {
			Name:                "衛星軌道2",
			McsRegion:           "best",
			EnableForceStartCmd: true,
			Cost630:             true,
			Comment:             "Cost 630",
		},
		21: {
			Name:                "サイド6宙域",
			McsRegion:           "best",
			EnableForceStartCmd: true,
			Comment:             "Private Room",
		},
		22: {
			Name:                "サイド7内部",
			McsRegion:           "best",
			EnableForceStartCmd: true,
			Comment:             "Private Room",
		},
	}

	for _, x := range lbsLobbySettings {
		locName, ok := gcpLocationName[x.McsRegion]
		if !ok {
			locName = "Default Server"
		}
		if x.McsRegion == "best" {
			locName = "Best Server [Auto Detection]"
		}
		x.Description = fmt.Sprintf("<B>%s<B><BR><B>%s<END>", locName, x.Comment)
	}
}
