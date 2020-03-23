package model

import (
	"net"
	"time"
)

type Battle struct {
	BattleCode string
	ServerIP   net.IP
	ServerPort uint16
	Users      []User
	AeugIDs    []string
	TitansIDs  []string
	UDPUsers   map[string]bool
	P2PMap     map[string]map[string]struct{}
	Rule       *Rule
	LobbyID    uint16
	StartTime  time.Time
	TestBattle bool
}

func NewBattle(lobbyID uint16) *Battle {
	return &Battle{
		Users:     make([]User, 0),
		AeugIDs:   make([]string, 0),
		TitansIDs: make([]string, 0),
		UDPUsers:  map[string]bool{},
		P2PMap:    map[string]map[string]struct{}{},
		Rule:      NewRule(),
		LobbyID:   lobbyID,
	}
}

func (b *Battle) SetRule(rule *Rule) {
	b.Rule = rule
}

func (b *Battle) Add(s *User) {
	cp := *s
	cp.Battle = nil
	cp.Lobby = nil
	cp.Room = nil
	b.Users = append(b.Users, cp)
	if s.Entry == EntryAeug {
		b.AeugIDs = append(b.AeugIDs, cp.UserID)
	} else if s.Entry == EntryTitans {
		b.TitansIDs = append(b.TitansIDs, cp.UserID)
	}
}

func (b *Battle) NumOfEntryUsers() uint16 {
	return uint16(len(b.AeugIDs) + len(b.TitansIDs))
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

func (b *Battle) GetUserByPos(pos byte) *User {
	pos -= 1
	if pos < 0 || len(b.Users) < int(pos) {
		return nil
	}
	return &b.Users[pos]
}

type BattleResult struct {
	Unk01       uint16 `json:"unk_01"`
	BattleCode  string `json:"battle_code"`
	Unk03       byte   `json:"unk_03"`
	Unk04       byte   `json:"unk_04"`
	Unk05       byte   `json:"unk_05"`
	Unk06       byte   `json:"unk_06"`
	BattleCount byte   `json:"battle_count"`
	WinCount    byte   `json:"win_count"`
	LoseCount   byte   `json:"lose_count"`
	KillCount   uint32 `json:"kill_count"`
	DeathCount  uint32 `json:"death_count"`
	TotalFrame  uint32 `json:"total_frame"`
	Unk13       uint32 `json:"unk_13"`
	Unk14       uint32 `json:"unk_14"`
	Side        byte   `json:"side"`
	Unk16       byte   `json:"unk_16"`
	Unk17       byte   `json:"unk_17"`
	Unk18       byte   `json:"unk_18"`
	Unk19       byte   `json:"unk_19"`
	Unk20       uint16 `json:"unk_20"`
	Unk21       uint16 `json:"unk_21"`
	Unk22       uint16 `json:"unk_22"`
	Unk23       uint16 `json:"unk_23"`
	Unk24       uint16 `json:"unk_24"`
	Unk25       uint16 `json:"unk_25"`
	Unk26       uint16 `json:"unk_26"`
	Unk27       uint16 `json:"unk_27"`
}
