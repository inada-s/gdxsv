package main

import (
	"bytes"
	"encoding/binary"
)

const (
	rulePlayerCount   = 0
	ruleBattleCount   = 1
	ruleVisibleTeam   = 2
	ruleBattleTime    = 3
	ruleBattleGauge   = 4
	ruleReloadType    = 5
	ruleDamageLevel   = 6
	ruleAwakeType     = 7
	ruleAwakeIncRatio = 8
	ruleAwakeDecRatio = 9
)

var ruleList []ruleElement
var defaultRule Rule

func init() {
	setupRuleList()
	setupDefaultRule()
}

type ruleElementElement struct {
	Name  string
	Value uint16
}

type ruleElement struct {
	Title   string
	Default byte
	List    []ruleElementElement
}

type Rule struct {
	playerCount   uint16
	battleCount   byte
	visibleTeam   byte
	battleTime    uint16
	battleGauge   uint16
	reloadType    byte
	damageLevel   byte
	awakeType     byte
	awakeIncRatio uint16
	awakeDecRatio uint16

	selected map[byte]byte
}

func NewRule() *Rule {
	r := defaultRule
	r.selected = make(map[byte]byte)
	return &r
}

func (r *Rule) Serialize() []byte {
	b := new(bytes.Buffer)
	var zero16 uint16
	var zero8 byte
	binary.Write(b, binary.BigEndian, uint16(100)) // Rule Length
	binary.Write(b, binary.BigEndian, zero16)      // Unknown
	binary.Write(b, binary.BigEndian, zero8)       // Unknown
	binary.Write(b, binary.BigEndian, zero8)       // Unknown
	for i := 0; i < 16*4; i++ {
		binary.Write(b, binary.BigEndian, byte(0xFF)) // Available MS Mask
	}
	binary.Write(b, binary.BigEndian, zero8) // MS/MA Auto Only
	binary.Write(b, binary.BigEndian, r.visibleTeam)
	binary.Write(b, binary.BigEndian, r.battleGauge)
	binary.Write(b, binary.BigEndian, r.battleGauge)
	binary.Write(b, binary.BigEndian, r.awakeIncRatio)
	binary.Write(b, binary.BigEndian, r.awakeDecRatio)
	binary.Write(b, binary.BigEndian, r.battleTime)
	binary.Write(b, binary.BigEndian, zero8) // Unknown
	binary.Write(b, binary.BigEndian, r.reloadType)
	binary.Write(b, binary.BigEndian, zero8) // Unknown
	binary.Write(b, binary.BigEndian, zero8) // Unknown
	binary.Write(b, binary.BigEndian, r.damageLevel-1)
	binary.Write(b, binary.BigEndian, r.battleCount)
	binary.Write(b, binary.BigEndian, zero16) // Unknown
	binary.Write(b, binary.BigEndian, zero16) // Unknown
	binary.Write(b, binary.BigEndian, zero16) // Unknown
	binary.Write(b, binary.BigEndian, zero16) // Unknown
	binary.Write(b, binary.BigEndian, r.awakeType)
	binary.Write(b, binary.BigEndian, zero8)     // Weapon Visible
	binary.Write(b, binary.BigEndian, zero16)    // Unknown
	binary.Write(b, binary.BigEndian, uint16(1)) // Enable ZZ Pilot
	return b.Bytes()
}

func (r *Rule) Set(ruleID, elemID byte) {
	r.selected[ruleID] = byte(elemID)
	value := ruleList[ruleID].List[elemID].Value
	switch ruleID {
	case rulePlayerCount:
		r.playerCount = value
	case ruleBattleCount:
		r.battleCount = byte(value)
	case ruleVisibleTeam:
		r.visibleTeam = byte(value)
	case ruleBattleTime:
		r.battleTime = value
	case ruleBattleGauge:
		r.battleGauge = value
	case ruleReloadType:
		r.reloadType = byte(value)
	case ruleDamageLevel:
		r.damageLevel = byte(value)
	case ruleAwakeType:
		r.awakeType = byte(value)
	case ruleAwakeIncRatio:
		r.awakeIncRatio = value
	case ruleAwakeDecRatio:
		r.awakeDecRatio = value
	}
}

func (r *Rule) Get(ruleID byte) byte {
	v, ok := r.selected[ruleID]
	if ok {
		return v
	}
	return ruleList[ruleID].Default
}

func RuleCount() byte {
	return byte(len(ruleList))
}

func RuleTitle(ruleID byte) string {
	return ruleList[ruleID].Title
}

func RuleElementCount(ruleID byte) byte {
	return byte(len(ruleList[ruleID].List))
}

func RuleElementName(ruleID, elemID byte) string {
	return ruleList[ruleID].List[elemID].Name
}

func RuleElementDefault(ruleID byte) byte {
	return ruleList[ruleID].Default
}

func setupRuleList() {
	ruleList = []ruleElement{
		{"人数設定", 2, []ruleElementElement{
			{"２人", 2}, {"３人", 3}, {"４人", 4}}},
		{"連続対戦数", 0, []ruleElementElement{
			{"任意", 0}, {"１戦", 1}, {"３戦", 3}, {"５戦", 5}, {"１０戦", 10}}},
		{"選択可視範囲", 0, []ruleElementElement{
			{"全員オープン", 0}, {"味方のみ", 1}, {"自分のみ", 2}}},
		{"作戦時間", 3, []ruleElementElement{
			{"９０秒", 90}, {"１２０秒", 120}, {"１８０秒", 180}, {"２１０秒", 210}, {"２７０秒", 270}}},
		{"戦力ゲージ", 2, []ruleElementElement{
			{"１", 1}, {"３００", 300}, {"６００", 600}, {"６１０", 610}, {"６２５", 625}, {"９００", 900}, {"１２００", 1200}}},
		{"リロード制限", 0, []ruleElementElement{
			{"通常", 0}, {"回復なし", 1}, {"制限なし", 2}}},
		{"ダメージレベル", 2, []ruleElementElement{
			{"１", 1}, {"２", 2}, {"３", 3}, {"４", 4}}},
		{"覚醒システム", 0, []ruleElementElement{
			{"任意", 0}, {"\"強襲\"固定", 1}, {"\"復活\"固定", 2}, {"\"機動\"固定", 3}, {"覚醒なし", 4}}},
		{"覚醒ゲージ増加量 ", 2, []ruleElementElement{
			{"１０％", 10}, {"５０％", 50}, {"１００％", 100}, {"１５０％", 150}, {"２００％", 200}}},
		{"覚醒継続時間", 2, []ruleElementElement{
			{"１０％", 10}, {"５０％", 50}, {"１００％", 100}, {"１５０％", 150}, {"２００％", 200}}},
	}
}

func setupDefaultRule() {
	defaultRule = Rule{
		playerCount:   ruleList[rulePlayerCount].List[ruleList[rulePlayerCount].Default].Value,
		battleCount:   byte(ruleList[ruleBattleCount].List[ruleList[ruleBattleCount].Default].Value),
		visibleTeam:   byte(ruleList[ruleVisibleTeam].List[ruleList[ruleVisibleTeam].Default].Value),
		battleTime:    ruleList[ruleBattleTime].List[ruleList[ruleBattleTime].Default].Value,
		battleGauge:   ruleList[ruleBattleGauge].List[ruleList[ruleBattleGauge].Default].Value,
		reloadType:    byte(ruleList[ruleReloadType].List[ruleList[ruleReloadType].Default].Value),
		damageLevel:   byte(ruleList[ruleDamageLevel].List[ruleList[ruleDamageLevel].Default].Value),
		awakeType:     byte(ruleList[ruleAwakeType].List[ruleList[ruleAwakeType].Default].Value),
		awakeIncRatio: ruleList[ruleAwakeIncRatio].List[ruleList[ruleAwakeIncRatio].Default].Value,
		awakeDecRatio: ruleList[ruleAwakeDecRatio].List[ruleList[ruleAwakeDecRatio].Default].Value,
	}
}
