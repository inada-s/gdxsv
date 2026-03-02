package main

import (
	"encoding/binary"
	"testing"
)

func TestSerializeRule_DefaultRule_ByteLength(t *testing.T) {
	// 14 byte fields + 2 uint16 fields (4 bytes) + 4 uint32 fields (16 bytes) + 1 byte field = 35 bytes
	// byte×14 + uint16×2 + uint32×4 + byte×1 = 14 + 4 + 16 + 1 = 35 but actually:
	// Counting from rule.go: 6 bytes + uint16×2(4) + 6 bytes + uint32×2(8) + 4 bytes + uint32×2(8) + 1 byte = 37
	// Let's count exactly:
	// byte(Difficulty) + byte(DamageLevel) + byte(Timer) + byte(TeamFlag) + byte(StageFlag) + byte(MsFlag) = 6
	// uint16(RenpoVital) + uint16(ZeonVital) = 4
	// byte(MaFlag) + byte(ReloadFlag) + byte(BoostKeep) + byte(RedarFlag) + byte(LockonFlag) + byte(Onematch) = 6
	// uint32(RenpoMaskPS2) + uint32(ZeonMaskPS2) = 8
	// byte(AutoRebattle) + byte(NoRanking) + byte(CPUFlag) + byte(SelectLook) = 4
	// uint32(RenpoMaskDC) + uint32(ZeonMaskDC) = 8
	// byte(StageNo) = 1
	// Total = 6 + 4 + 6 + 8 + 4 + 8 + 1 = 37
	const expectedLen = 37

	rule := DefaultRule
	b := SerializeRule(&rule)
	assertEq(t, expectedLen, len(b))
}

func TestSerializeRule_FieldOffsets(t *testing.T) {
	rule := Rule{
		Difficulty:   3,
		DamageLevel:  2,
		Timer:        3,
		TeamFlag:     0,
		StageFlag:    1,
		MsFlag:       1,
		RenpoVital:   600,
		ZeonVital:    500,
		MaFlag:       1,
		ReloadFlag:   0,
		BoostKeep:    0,
		RedarFlag:    1,
		LockonFlag:   0,
		Onematch:     1,
		RenpoMaskPS2: 0xAABBCCDD,
		ZeonMaskPS2:  0x11223344,
		AutoRebattle: 1,
		NoRanking:    0,
		CPUFlag:      0xff,
		SelectLook:   1,
		RenpoMaskDC:  0x55667788,
		ZeonMaskDC:   0x99AABBCC,
		StageNo:      5,
	}
	b := SerializeRule(&rule)

	// byte fields at offsets 0-5
	assertEq(t, byte(3), b[0])  // Difficulty
	assertEq(t, byte(2), b[1])  // DamageLevel
	assertEq(t, byte(3), b[2])  // Timer
	assertEq(t, byte(0), b[3])  // TeamFlag
	assertEq(t, byte(1), b[4])  // StageFlag
	assertEq(t, byte(1), b[5])  // MsFlag

	// uint16 fields at offsets 6-9 (little-endian)
	assertEq(t, uint16(600), binary.LittleEndian.Uint16(b[6:8]))  // RenpoVital
	assertEq(t, uint16(500), binary.LittleEndian.Uint16(b[8:10])) // ZeonVital

	// byte fields at offsets 10-15
	assertEq(t, byte(1), b[10]) // MaFlag
	assertEq(t, byte(0), b[11]) // ReloadFlag
	assertEq(t, byte(0), b[12]) // BoostKeep
	assertEq(t, byte(1), b[13]) // RedarFlag
	assertEq(t, byte(0), b[14]) // LockonFlag
	assertEq(t, byte(1), b[15]) // Onematch

	// uint32 fields at offsets 16-23 (little-endian)
	assertEq(t, uint32(0xAABBCCDD), binary.LittleEndian.Uint32(b[16:20])) // RenpoMaskPS2
	assertEq(t, uint32(0x11223344), binary.LittleEndian.Uint32(b[20:24])) // ZeonMaskPS2

	// byte fields at offsets 24-27
	assertEq(t, byte(1), b[24])    // AutoRebattle
	assertEq(t, byte(0), b[25])    // NoRanking
	assertEq(t, byte(0xff), b[26]) // CPUFlag
	assertEq(t, byte(1), b[27])    // SelectLook

	// uint32 fields at offsets 28-35 (little-endian)
	assertEq(t, uint32(0x55667788), binary.LittleEndian.Uint32(b[28:32])) // RenpoMaskDC
	assertEq(t, uint32(0x99AABBCC), binary.LittleEndian.Uint32(b[32:36])) // ZeonMaskDC

	// byte field at offset 36
	assertEq(t, byte(5), b[36]) // StageNo
}

func TestSerializeRule_ZeroValue(t *testing.T) {
	rule := Rule{}
	b := SerializeRule(&rule)
	assertEq(t, 37, len(b))

	// All bytes should be zero
	for i, v := range b {
		if v != 0 {
			t.Errorf("byte at offset %d = %d, want 0", i, v)
		}
	}
}

func TestSerializeRule_DefaultRule_Values(t *testing.T) {
	rule := DefaultRule
	b := SerializeRule(&rule)

	assertEq(t, byte(3), b[0])                                            // Difficulty
	assertEq(t, byte(2), b[1])                                            // DamageLevel
	assertEq(t, byte(3), b[2])                                            // Timer
	assertEq(t, uint16(600), binary.LittleEndian.Uint16(b[6:8]))          // RenpoVital
	assertEq(t, uint16(600), binary.LittleEndian.Uint16(b[8:10]))         // ZeonVital
	assertEq(t, byte(1), b[5])                                            // MsFlag
	assertEq(t, uint32(0xffffffff), binary.LittleEndian.Uint32(b[16:20])) // RenpoMaskPS2
	assertEq(t, uint32(0xffffffff), binary.LittleEndian.Uint32(b[20:24])) // ZeonMaskPS2
	assertEq(t, byte(0xff), b[26])                                        // CPUFlag
	assertEq(t, byte(1), b[27])                                           // SelectLook
	assertEq(t, uint32(0xffffffff), binary.LittleEndian.Uint32(b[28:32])) // RenpoMaskDC
	assertEq(t, uint32(0xffffffff), binary.LittleEndian.Uint32(b[32:36])) // ZeonMaskDC
}
