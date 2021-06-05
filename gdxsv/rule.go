package main

import (
	"bytes"
	"encoding/binary"
)

type Rule MRule

var (
	DefaultRule Rule
)

func init() {
	DefaultRule = baseRule
}

var baseRule = Rule{
	Difficulty:   3,   // Game Difficulty (zero-indexed)
	DamageLevel:  2,   // Game DamageLevel (zero-indexed)
	Timer:        3,   // 2:180sec 3:210sec
	TeamFlag:     0,   // 1:team select (buggy)
	StageFlag:    0,   // 0:side7 1:ground 2:space 3:ground and space
	MsFlag:       1,   // 1:opponent team MS available
	RenpoVital:   600, // renpo total cost
	ZeonVital:    600, // zeon total cost
	MaFlag:       0,   // 1:MA available
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

func SerializeRule(r *Rule) []byte {
	b := new(bytes.Buffer)
	_ = binary.Write(b, binary.LittleEndian, byte(r.Difficulty))
	_ = binary.Write(b, binary.LittleEndian, byte(r.DamageLevel))
	_ = binary.Write(b, binary.LittleEndian, byte(r.Timer))
	_ = binary.Write(b, binary.LittleEndian, byte(r.TeamFlag))
	_ = binary.Write(b, binary.LittleEndian, byte(r.StageFlag))
	_ = binary.Write(b, binary.LittleEndian, byte(r.MsFlag))
	_ = binary.Write(b, binary.LittleEndian, uint16(r.RenpoVital))
	_ = binary.Write(b, binary.LittleEndian, uint16(r.ZeonVital))
	_ = binary.Write(b, binary.LittleEndian, byte(r.MaFlag))
	_ = binary.Write(b, binary.LittleEndian, byte(r.ReloadFlag))
	_ = binary.Write(b, binary.LittleEndian, byte(r.BoostKeep))
	_ = binary.Write(b, binary.LittleEndian, byte(r.RedarFlag))
	_ = binary.Write(b, binary.LittleEndian, byte(r.LockonFlag))
	_ = binary.Write(b, binary.LittleEndian, byte(r.Onematch))
	_ = binary.Write(b, binary.LittleEndian, uint32(r.RenpoMaskPS2))
	_ = binary.Write(b, binary.LittleEndian, uint32(r.ZeonMaskPS2))
	_ = binary.Write(b, binary.LittleEndian, byte(r.AutoRebattle))
	_ = binary.Write(b, binary.LittleEndian, byte(r.NoRanking))
	_ = binary.Write(b, binary.LittleEndian, byte(r.CPUFlag))
	_ = binary.Write(b, binary.LittleEndian, byte(r.SelectLook))
	_ = binary.Write(b, binary.LittleEndian, uint32(r.RenpoMaskDC))
	_ = binary.Write(b, binary.LittleEndian, uint32(r.ZeonMaskDC))
	_ = binary.Write(b, binary.LittleEndian, byte(r.StageNo))
	return b.Bytes()
}
