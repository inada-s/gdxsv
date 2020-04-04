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
	AtuoRebattle byte
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
	Timer:        2,
	TeamFlag:     1,
	StageFlag:    3,
	MsFlag:       11,
	RenpoVital:   600,
	ZeonVital:    600,
	MaFlag:       1,
	ReloadFlag:   0,
	BoostKeep:    0,
	RedarFlag:    0,
	LockonFlag:   0,
	Onematch:     0,
	RenpoMask:    0x00fffa0f,
	ZeonMask:     0x00fffa0f,
	AtuoRebattle: 0,
	NoRanking:    0,
	CPUFlag:      1,
	SelectLook:   0xff,
	Unused1:      0,
	Unused2:      0,
	Unused3:      0,
	Unused4:      0,
	TeamType0:    0,
	TeamType1:    0,
	TeamType2:    1,
	TeamType3:    1,
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
	binary.Write(b, binary.LittleEndian, r.AtuoRebattle)
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
