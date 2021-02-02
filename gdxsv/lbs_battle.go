package main

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"go.uber.org/zap"
)

func genBattleCode() string {
	return fmt.Sprintf("%013d", time.Now().UnixNano()/1000000)
}

type LbsBattle struct {
	app *Lbs

	BattleCode string
	McsRegion  string
	ServerIP   net.IP
	ServerPort uint16
	Users      []*DBUser
	UserRanks  []int
	RenpoIDs   []string
	ZeonIDs    []string
	GameParams [][]byte
	Rule       *Rule
	LobbyID    uint16
	StartTime  time.Time
	TestBattle bool
}

func splitIPPort(addr string) (net.IP, uint16, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, 0, err
	}

	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, 0, err
	}

	return net.ParseIP(host), uint16(portNum), nil
}

func NewBattle(app *Lbs, lobbyID uint16, rule *Rule, mcsRegion string, mcsAddr string) *LbsBattle {
	if mcsAddr == "" {
		mcsRegion = ""
		mcsAddr = conf.BattlePublicAddr
	}

	ip, port, err := splitIPPort(mcsAddr)
	if err != nil {
		logger.Error("failed to parse mcs addr", zap.Error(err))
		return nil
	}

	if rule == nil {
		rule = &DefaultRule
	}

	return &LbsBattle{
		app: app,

		BattleCode: genBattleCode(),
		McsRegion:  mcsRegion,
		ServerIP:   ip,
		ServerPort: port,
		Users:      make([]*DBUser, 0),
		UserRanks:  make([]int, 0),
		GameParams: make([][]byte, 0),
		RenpoIDs:   make([]string, 0),
		ZeonIDs:    make([]string, 0),
		Rule:       rule,
		LobbyID:    lobbyID,
	}
}

func (b *LbsBattle) SetRule(rule *Rule) {
	b.Rule = rule
}

func (b *LbsBattle) Add(p *LbsPeer) {
	b.Users = append(b.Users, &p.DBUser)
	b.GameParams = append(b.GameParams, p.GameParam)
	b.UserRanks = append(b.UserRanks, p.Rank)
	if p.Team == TeamRenpo {
		b.RenpoIDs = append(b.RenpoIDs, p.UserID)
	} else if p.Team == TeamZeon {
		b.ZeonIDs = append(b.ZeonIDs, p.UserID)
	}
}

func (b *LbsBattle) NumOfEntryUsers() uint16 {
	return uint16(len(b.RenpoIDs) + len(b.ZeonIDs))
}

func (b *LbsBattle) SetBattleServer(addr string) {
	ip, port, err := splitIPPort(addr)
	if err != nil {
		logger.Error("failed to set battle server", zap.Error(err))
		return
	}
	b.ServerIP = ip
	b.ServerPort = port
}

func (b *LbsBattle) GetPosition(userID string) byte {
	for i, u := range b.Users {
		if userID == u.UserID {
			return byte(i + 1)
		}
	}
	return 0
}

func (b *LbsBattle) GetUserByPos(pos byte) *DBUser {
	pos--
	if pos < 0 || len(b.Users) < int(pos) {
		return nil
	}
	return b.Users[pos]
}

func (b *LbsBattle) GetGameParamByPos(pos byte) []byte {
	pos--
	if pos < 0 || len(b.GameParams) < int(pos) {
		return nil
	}
	return b.GameParams[pos]
}

func (b *LbsBattle) GetUserRankByPos(pos byte) int {
	pos--
	if pos < 0 || len(b.UserRanks) < int(pos) {
		return 0
	}
	return b.UserRanks[pos]
}

func (b *LbsBattle) GetUserTeam(userID string) uint16 {
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
	WinCount    byte   `json:"win_count,omitempty"`
	LoseCount   byte   `json:"lose_count,omitempty"`
	Unk6        byte   `json:"unk_6,omitempty"`
	Unk7        byte   `json:"unk_7,omitempty"`
	Unk8        uint32 `json:"unk_8,omitempty"`
	Unk9        byte   `json:"unk_9,omitempty"`
	Unk10       byte   `json:"unk_10,omitempty"`
	Unk11       byte   `json:"unk_11,omitempty"`
	Unk12       byte   `json:"unk_12,omitempty"`
	KillCount   uint16 `json:"kill_count,omitempty"`
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
