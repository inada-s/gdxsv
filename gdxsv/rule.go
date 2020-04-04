package main

import (
	"bytes"
	"encoding/binary"
)

type Rule struct {
	Difficulty   byte
	DamageLevel  byte
	Timer        byte
	TeamFlag     byte
	StageFlag    byte
	MsFlag       byte
	RenpoVital   uint16
	ZeonVital    uint16
	MaFlag       byte
	ReloadFlag   byte
	BoostKeep    byte
	RedarFlag    byte
	LockonFlag   byte
	Onematch     byte
	RenpoMask    uint32
	ZeonMask     uint32
	AutoRebattle byte
	NoRanking    byte
	CPUFlag      byte
	SelectLook   byte
	Unused1      byte
	Unused2      byte
	Unused3      byte
	Unused4      byte
	TeamType0    byte
	TeamType1    byte
	TeamType2    byte
	TeamType3    byte
	StageNo      byte
}

var DefaultRule = Rule{
	Difficulty:   3,
	DamageLevel:  2,
	Timer:        2,   // 2:180sec
	TeamFlag:     1,   // 1:side select (buggy)
	StageFlag:    3,   // 1:ground 2:space 3:ground and space
	MsFlag:       1,   // 1:opponent side MS available
	RenpoVital:   600, // renpo total cost
	ZeonVital:    600, // zeon total cost
	MaFlag:       1,   // 1:opponent side MA available
	ReloadFlag:   0,   // 1:unlimited ammo
	BoostKeep:    0,   // unknown
	RedarFlag:    0,   // 1:no rader
	LockonFlag:   0,   // 1:disable lockon warning
	Onematch:     0,
	RenpoMask:    0xffffffff,
	ZeonMask:     0xffffffff,
	AutoRebattle: 0,
	NoRanking:    0, // 1:no battle record
	CPUFlag:      0, // unknown
	SelectLook:   0, // 1:can see enemy's choice of MS
	Unused1:      0,
	Unused2:      0,
	Unused3:      0,
	Unused4:      0,
	TeamType0:    0, // unknown
	TeamType1:    0, // unknown
	TeamType2:    1, // unknown
	TeamType3:    1, // unknown
	StageNo:      0,
}

func NewRule() *Rule {
	r := DefaultRule // copy
	return &r
}

func (r *Rule) Serialize() []byte {
	b := new(bytes.Buffer)
	binary.Write(b, binary.LittleEndian, r.Difficulty)
	binary.Write(b, binary.LittleEndian, r.DamageLevel)
	binary.Write(b, binary.LittleEndian, r.Timer)
	binary.Write(b, binary.LittleEndian, r.TeamFlag)
	binary.Write(b, binary.LittleEndian, r.StageFlag)
	binary.Write(b, binary.LittleEndian, r.MsFlag)
	binary.Write(b, binary.LittleEndian, r.RenpoVital)
	binary.Write(b, binary.LittleEndian, r.ZeonVital)
	binary.Write(b, binary.LittleEndian, r.MaFlag)
	binary.Write(b, binary.LittleEndian, r.ReloadFlag)
	binary.Write(b, binary.LittleEndian, r.BoostKeep)
	binary.Write(b, binary.LittleEndian, r.RedarFlag)
	binary.Write(b, binary.LittleEndian, r.LockonFlag)
	binary.Write(b, binary.LittleEndian, r.Onematch)
	binary.Write(b, binary.LittleEndian, r.RenpoMask)
	binary.Write(b, binary.LittleEndian, r.ZeonMask)
	binary.Write(b, binary.LittleEndian, r.AutoRebattle)
	binary.Write(b, binary.LittleEndian, r.NoRanking)
	binary.Write(b, binary.LittleEndian, r.CPUFlag)
	binary.Write(b, binary.LittleEndian, r.SelectLook)
	binary.Write(b, binary.LittleEndian, r.Unused1)
	binary.Write(b, binary.LittleEndian, r.Unused2)
	binary.Write(b, binary.LittleEndian, r.Unused3)
	binary.Write(b, binary.LittleEndian, r.Unused4)
	binary.Write(b, binary.LittleEndian, r.TeamType0)
	binary.Write(b, binary.LittleEndian, r.TeamType1)
	binary.Write(b, binary.LittleEndian, r.TeamType2)
	binary.Write(b, binary.LittleEndian, r.TeamType3)
	binary.Write(b, binary.LittleEndian, r.StageNo)
	return b.Bytes()
}
