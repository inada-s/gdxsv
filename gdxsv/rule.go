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
	RenpoMaskPS2 uint32
	ZeonMaskPS2  uint32
	AutoRebattle byte
	NoRanking    byte
	CPUFlag      byte
	SelectLook   byte
	RenpoMaskDC  uint32
	ZeonMaskDC   uint32
	StageNo      byte
}

var DefaultRule = Rule{
	Difficulty:   3,
	DamageLevel:  2,
	Timer:        2,   // 2:180sec
	TeamFlag:     0,   // 1:side select (buggy)
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
	RenpoMaskPS2: 0xffffffff,
	ZeonMaskPS2:  0xffffffff,
	AutoRebattle: 0,
	NoRanking:    0,    // 1:no battle record
	CPUFlag:      0xff, // unknown
	SelectLook:   1,    // 1:can see opponent's MS choice
	RenpoMaskDC:  0xffffffff,
	ZeonMaskDC:   0xffffffff,
	StageNo:      0, // unknown
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
	binary.Write(b, binary.LittleEndian, r.RenpoMaskPS2)
	binary.Write(b, binary.LittleEndian, r.ZeonMaskPS2)
	binary.Write(b, binary.LittleEndian, r.AutoRebattle)
	binary.Write(b, binary.LittleEndian, r.NoRanking)
	binary.Write(b, binary.LittleEndian, r.CPUFlag)
	binary.Write(b, binary.LittleEndian, r.SelectLook)
	binary.Write(b, binary.LittleEndian, r.RenpoMaskDC)
	binary.Write(b, binary.LittleEndian, r.ZeonMaskDC)
	binary.Write(b, binary.LittleEndian, r.StageNo)
	return b.Bytes()
}
