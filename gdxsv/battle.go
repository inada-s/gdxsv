package main

import (
	"net"
	"time"
)

type Battle struct {
	app *App

	BattleCode string
	ServerIP   net.IP
	ServerPort uint16
	Users      []*DBUser
	RenpoIDs   []string
	ZeonIDs    []string
	GameParams [][]byte
	Rule       *Rule
	LobbyID    uint16
	StartTime  time.Time
	TestBattle bool
}

func NewBattle(app *App, lobbyID uint16) *Battle {
	return &Battle{
		app: app,

		Users:      make([]*DBUser, 0),
		GameParams: make([][]byte, 0),
		RenpoIDs:   make([]string, 0),
		ZeonIDs:    make([]string, 0),
		Rule:       NewRule(),
		LobbyID:    lobbyID,
	}
}

func (b *Battle) SetRule(rule *Rule) {
	b.Rule = rule
}

func (b *Battle) Add(p *AppPeer) {
	b.Users = append(b.Users, &p.DBUser)
	b.GameParams = append(b.GameParams, p.GameParam)
	if p.Entry == EntryRenpo {
		b.RenpoIDs = append(b.RenpoIDs, p.UserID)
	} else if p.Entry == EntryZeon {
		b.ZeonIDs = append(b.ZeonIDs, p.UserID)
	}
}

func (b *Battle) NumOfEntryUsers() uint16 {
	return uint16(len(b.RenpoIDs) + len(b.ZeonIDs))
}

func (b *Battle) SetBattleServer(ip net.IP, port uint16) {
	b.ServerIP = ip
	b.ServerPort = port
}

func (b *Battle) GetPosition(userID string) byte {
	for i, u := range b.Users {
		if userID == u.UserID {
			return byte(i + 1)
		}
	}
	return 0
}

func (b *Battle) GetUserByPos(pos byte) *DBUser {
	pos -= 1
	if pos < 0 || len(b.Users) < int(pos) {
		return nil
	}
	return b.Users[pos]
}

func (b *Battle) GetGameParamByPos(pos byte) []byte {
	pos -= 1
	if pos < 0 || len(b.Users) < int(pos) {
		return nil
	}
	return b.GameParams[pos]
}

func (b *Battle) GetUserSide(userID string) uint16 {
	for _, id := range b.RenpoIDs {
		if id == userID {
			return 1
		}
	}
	for _, id := range b.ZeonIDs {
		if id == userID {
			return 2
		}
	}
	return 0
}

type BattleResult struct {
	BattleCode  string `json:"battle_code,omitempty"`
	Unk2        byte   `json:"unk_2,omitempty"`
	BattleCount byte   `json:"battle_count,omitempty"`
	Unk4        byte   `json:"unk_4,omitempty"`
	Unk5        byte   `json:"unk_5,omitempty"`
	Unk6        byte   `json:"unk_6,omitempty"`
	Unk7        byte   `json:"unk_7,omitempty"`
	Unk8        uint32 `json:"unk_8,omitempty"`
	Unk9        byte   `json:"unk_9,omitempty"`
	Unk10       byte   `json:"unk_10,omitempty"`
	Unk11       byte   `json:"unk_11,omitempty"`
	Unk12       byte   `json:"unk_12,omitempty"`
	Unk13       uint16 `json:"unk_13,omitempty"`
	Unk14       uint16 `json:"unk_14,omitempty"`
	Unk15       uint16 `json:"unk_15,omitempty"`
	Unk16       uint16 `json:"unk_16,omitempty"`
	Unk17       uint16 `json:"unk_17,omitempty"`
	Unk18       uint16 `json:"unk_18,omitempty"`
	Unk19       uint16 `json:"unk_19,omitempty"`
	Unk20       uint16 `json:"unk_20,omitempty"`
	Unk21       uint16 `json:"unk_21,omitempty"`
	Unk22       uint16 `json:"unk_22,omitempty"`
	Unk23       uint16 `json:"unk_23,omitempty"`
	Unk24       uint16 `json:"unk_24,omitempty"`
	Unk25       uint16 `json:"unk_25,omitempty"`
	Unk26       uint16 `json:"unk_26,omitempty"`
	Unk27       uint16 `json:"unk_27,omitempty"`
	Unk28       uint16 `json:"unk_28,omitempty"`
}
