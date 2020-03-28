package main

import (
	"net"
	"time"
)

type Battle struct {
	BattleCode string
	ServerIP   net.IP
	ServerPort uint16
	Users      []*AppPeer
	RenpoIDs   []string
	ZeonIDs    []string
	UDPUsers   map[string]bool
	P2PMap     map[string]map[string]struct{}
	Rule       *Rule
	LobbyID    uint16
	StartTime  time.Time
	TestBattle bool
}

func NewBattle(lobbyID uint16) *Battle {
	return &Battle{
		Users:    make([]*AppPeer, 0),
		RenpoIDs: make([]string, 0),
		ZeonIDs:  make([]string, 0),
		UDPUsers: map[string]bool{},
		P2PMap:   map[string]map[string]struct{}{},
		Rule:     NewRule(),
		LobbyID:  lobbyID,
	}
}

func (b *Battle) SetRule(rule *Rule) {
	b.Rule = rule
}

func (b *Battle) Add(peer *AppPeer) {
	b.Users = append(b.Users, peer)
	if peer.Entry == EntryRenpo {
		b.RenpoIDs = append(b.RenpoIDs, peer.UserID)
	} else if peer.Entry == EntryZeon {
		b.ZeonIDs = append(b.ZeonIDs, peer.UserID)
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

func (b *Battle) GetUserByPos(pos byte) *AppPeer {
	pos -= 1
	if pos < 0 || len(b.Users) < int(pos) {
		return nil
	}
	return b.Users[pos]
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
