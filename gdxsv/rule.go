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

// DC MSBitMask
// I investigated with the DC2 version. Not verified elsewhere.
const MSMaskAll uint = 0xffffffff

const (
	MSMaskDCGundam uint = 1 << iota
	MSMaskDCGuncannon
	MSMaskDCGM
	MSMaskDCZaku1
	MSMaskDCZaku2
	MSMaskDCZaku2S
	MSMaskDCGouf
	MSMaskDCDom
	MSMaskDCRickDom
	MSMaskDCGelgoog
	MSMaskDCGelgoogS
	MSMaskDCGyan
	MSMaskDCGogg
	MSMaskDCAcguy
	MSMaskDCZgok
	MSMaskDCZgokS
	MSMaskDCZock
	MSMaskDCGuntank
	MSMaskDCZeong
	MSMaskDCLGundam
	MSMaskDCLGM
	MSMaskDCElmeth
	MSMaskDCBall
	MSMaskDCBrawBro
	MSMaskDCDUMMY24
	MSMaskDCZakrello
	MSMaskDCBigro
	MSMaskDCBigZam
	MSMaskDCAdzam
	MSMaskDCGFighter
)

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
	RenpoMaskDC:  MSMaskAll,
	ZeonMaskDC:   MSMaskAll,
	StageNo:      0, // unknown
}

func SerializeRule(r *Rule) []byte {
	b := new(bytes.Buffer)
	binary.Write(b, binary.LittleEndian, byte(r.Difficulty))
	binary.Write(b, binary.LittleEndian, byte(r.DamageLevel))
	binary.Write(b, binary.LittleEndian, byte(r.Timer))
	binary.Write(b, binary.LittleEndian, byte(r.TeamFlag))
	binary.Write(b, binary.LittleEndian, byte(r.StageFlag))
	binary.Write(b, binary.LittleEndian, byte(r.MsFlag))
	binary.Write(b, binary.LittleEndian, uint16(r.RenpoVital))
	binary.Write(b, binary.LittleEndian, uint16(r.ZeonVital))
	binary.Write(b, binary.LittleEndian, byte(r.MaFlag))
	binary.Write(b, binary.LittleEndian, byte(r.ReloadFlag))
	binary.Write(b, binary.LittleEndian, byte(r.BoostKeep))
	binary.Write(b, binary.LittleEndian, byte(r.RedarFlag))
	binary.Write(b, binary.LittleEndian, byte(r.LockonFlag))
	binary.Write(b, binary.LittleEndian, byte(r.Onematch))
	binary.Write(b, binary.LittleEndian, uint32(r.RenpoMaskPS2))
	binary.Write(b, binary.LittleEndian, uint32(r.ZeonMaskPS2))
	binary.Write(b, binary.LittleEndian, byte(r.AutoRebattle))
	binary.Write(b, binary.LittleEndian, byte(r.NoRanking))
	binary.Write(b, binary.LittleEndian, byte(r.CPUFlag))
	binary.Write(b, binary.LittleEndian, byte(r.SelectLook))
	binary.Write(b, binary.LittleEndian, uint32(r.RenpoMaskDC))
	binary.Write(b, binary.LittleEndian, uint32(r.ZeonMaskDC))
	binary.Write(b, binary.LittleEndian, byte(r.StageNo))
	return b.Bytes()
}
