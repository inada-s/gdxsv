package main

import (
	"bytes"
	"encoding/binary"
)

var (
	RulePresetDefault   *Rule
	RulePresetNoRanking *Rule
	RulePresetNo375MS   *Rule
	RulePresetExtraCost *Rule
)

func init() {
	RulePresetDefault = baseRule.Clone()

	RulePresetNoRanking = baseRule.Clone()
	RulePresetNoRanking.NoRanking = 1
	RulePresetNoRanking.StageFlag = 3
	RulePresetNoRanking.MaFlag = 1

	RulePresetNo375MS = baseRule.Clone()
	RulePresetNo375MS.RenpoMaskDC = MSMaskAll & ^MSMaskDCGundam & ^MSMaskDCGelgoogS & ^MSMaskDCZeong & ^MSMaskDCElmeth
	RulePresetNo375MS.ZeonMaskDC = MSMaskAll & ^MSMaskDCGundam & ^MSMaskDCGelgoogS & ^MSMaskDCZeong & ^MSMaskDCElmeth

	RulePresetExtraCost = baseRule.Clone()
	RulePresetExtraCost.NoRanking = 1
	RulePresetExtraCost.RenpoVital = 630
	RulePresetExtraCost.ZeonVital = 630
}

// DC MSBitMask
// I investigated with the DC2 version. Not verified elsewhere.
const MSMaskAll uint32 = 0xffffffff

const (
	MSMaskDCGundam uint32 = 1 << iota
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

type Rule struct {
	Difficulty   byte   `json:"difficulty,omitempty"`
	DamageLevel  byte   `json:"damage_level,omitempty"`
	Timer        byte   `json:"timer,omitempty"`
	TeamFlag     byte   `json:"team_flag,omitempty"`
	StageFlag    byte   `json:"stage_flag,omitempty"`
	MsFlag       byte   `json:"ms_flag,omitempty"`
	RenpoVital   uint16 `json:"renpo_vital,omitempty"`
	ZeonVital    uint16 `json:"zeon_vital,omitempty"`
	MaFlag       byte   `json:"ma_flag,omitempty"`
	ReloadFlag   byte   `json:"reload_flag,omitempty"`
	BoostKeep    byte   `json:"boost_keep,omitempty"`
	RedarFlag    byte   `json:"redar_flag,omitempty"`
	LockonFlag   byte   `json:"lockon_flag,omitempty"`
	Onematch     byte   `json:"onematch,omitempty"`
	RenpoMaskPS2 uint32 `json:"renpo_mask_ps_2,omitempty"`
	ZeonMaskPS2  uint32 `json:"zeon_mask_ps_2,omitempty"`
	AutoRebattle byte   `json:"auto_rebattle,omitempty"`
	NoRanking    byte   `json:"no_ranking,omitempty"`
	CPUFlag      byte   `json:"cpu_flag,omitempty"`
	SelectLook   byte   `json:"select_look,omitempty"`
	RenpoMaskDC  uint32 `json:"renpo_mask_dc,omitempty"`
	ZeonMaskDC   uint32 `json:"zeon_mask_dc,omitempty"`
	StageNo      byte   `json:"stage_no,omitempty"`
}

var baseRule = &Rule{
	Difficulty:   3,   // Game Difficulty (zero-indexed)
	DamageLevel:  2,   // Game DamageLevel (zero-indexed)
	Timer:        3,   // 2:180sec 3:210sec
	TeamFlag:     0,   // 1:side select (buggy)
	StageFlag:    0,   // 0:side7 1:ground 2:space 3:ground and space
	MsFlag:       1,   // 1:opponent side MS available
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

func (r *Rule) Clone() *Rule {
	s := *r
	return &s
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
