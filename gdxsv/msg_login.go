package main

import (
	"fmt"

	"github.com/golang/glog"
)

const (
	// 0035b5e0 : lobby_act_00_01
	CMD_LoginType       = 0x6110 // Key
	CMD_ConnectionId    = 0x6101
	CMD_AskConnectionId = 0x6102
	CMD_WarningMessage  = 0x6103 // Key

	// 0035b8f0 : lobby_act_00_02
	CMD_RegulationHeader = 0x6820
	CMD_RegulationText   = 0x6821
	CMD_RegulationFooter = 0x6822

	// 0035bd10 : lobby_act_00_03
	// User Personal Info (Pending)

	// 0035bf80 : lobby_act_00_04
	// 0035c230 : lobby_act_00_05
	// 0035c400 : lobby_act_00_06
	// Select/Register User ID
	CMD_UserHandle = 0x6111
	CMD_UserRegist = 0x6112

	// 0035c5cc : lobby_act_00_07
	CMD_AddProgress     = 0x6118
	CMD_AskBattleResult = 0x6120
	CMD_AskGameVersion  = 0x6117
	CMD_AskGameCode     = 0x6116
	CMD_AskCountryCode  = 0x6115
	CMD_AskPlatformCode = 0x6114

	// 0035c750 : lobby_act_00_08
	CMD_KDDICharges   = 0x6142
	CMD_GameParameter = 0x6143
	CMD_WinLose       = 0x6145
	CMD_RankRanking   = 0x6144
	CMD_ServerMoney   = 0x6149

	CMD_NewsTag       = 0x6801
	CMD_NewsText      = 0x6802
	CMD_InvitationTag = 0x6810

	CMD_TopRankingTag = 0x6851

	CMD_PatchData_6861       = 0x6861 // 003691b0 Send_Req_PatchData
	CMD_PatchHeader          = 0x6862
	CMD_PatchData_6863       = 0x6863
	CMD_CalcDownloadChecksum = 0x6864
	CMD_PatchPing            = 0x6865

	// lobby_sub_01_03
	CMD_PlazaMax     = 0x6203
	CMD_PlazaTitle   = 0x6204 // UNUSED
	CMD_PlazaJoin    = 0x6205
	CMD_PlazaStatus  = 0x6206
	CMD_PlazaExplain = 0x620a
)

func StartLoginFlow(p *AppPeer) {
	p.SendMessage(NewServerQuestion(CMD_AskConnectionId))
}

var _ = register(CMD_AskConnectionId, "ConnectionId", func(p *AppPeer, m *Message) {
	connID := m.Reader().ReadString()
	glog.Infoln("CMD_AskConnectionId", connID)
	if connID == "" {
		connID = "abc123"
	}
	p.SendMessage(NewServerQuestion(CMD_ConnectionId).Writer().WriteString(connID).Msg())
})

var _ = register(CMD_ConnectionId, "ConnectionId", func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerNotice(CMD_WarningMessage).Writer().Write8(0).Msg())
})

var _ = register(CMD_RegulationHeader, "RegurationHeader", func(p *AppPeer, m *Message) {
	glog.Infoln("RegurationHeader")
	p.SendMessage(NewServerAnswer(m).Writer().WriteString("1000").WriteString("1000").Msg())
	p.SendMessage(NewServerNotice(CMD_RegulationText).Writer().WriteString("tag").WriteString("text").Msg())
	p.SendMessage(NewServerNotice(CMD_RegulationFooter))
	p.SendMessage(NewServerQuestion(CMD_LoginType))
})

var _ = register(CMD_LoginType, "LoginType", func(p *AppPeer, m *Message) {
	glog.Infoln("LoginType", m.Reader().Read8())

	// FIXME: I think it is wrong.
	a := NewServerNotice(CMD_UserHandle)
	w := a.Writer()
	w.Write8(1) // number of user id
	w.WriteString("GDXSV_")
	w.WriteString("ハンドルネーム")
	p.SendMessage(a)
})

var _ = register(CMD_UserRegist, "UserRegist", func(p *AppPeer, m *Message) {
	r := m.Reader()
	userID := r.ReadString() // ******
	handleName := r.ReadString()
	glog.Infoln("UserRegist", userID, handleName)
	p.SendMessage(NewServerAnswer(m).Writer().WriteString("NEWUSR").Msg()) // right?
})

var _ = register(0x6113, "StartLogin", func(p *AppPeer, m *Message) {
	userID := m.Reader().ReadString()
	glog.Infoln("DecideUserId", userID)
	p.SendMessage(NewServerAnswer(m).Writer().WriteString(userID).Msg())

	p.SendMessage(NewServerNotice(CMD_AddProgress)) // right?
})

var _ = register(CMD_PatchData_6861, "PatchData_6861", func(p *AppPeer, m *Message) {
	r := m.Reader()
	platform := r.Read8()
	crule := r.Read8()
	data := r.ReadString()
	_ = platform
	_ = crule
	_ = data
	p.SendMessage(NewServerAnswer(m))
	p.SendMessage(NewServerNotice(CMD_ServerMoney).Writer().Write16(123).Write16(456).Write16(789).Write16(101).Msg())
})
var _ = register(CMD_InvitationTag, "InvitationTag", func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().WriteString("tabbuf").WriteString("invitation").Write8(1).Msg())
})

var _ = register(CMD_PlazaMax, "PlazaMax", func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().Write16(23).Msg())
})

/*
var _ = register(CMD_PlazaTitle, "PlazaTitle", func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().Write16(lobbyID).WriteString(fmt.Sprint("LobbyTitle", lobbyID)).Msg())
})
*/

var _ = register(CMD_PlazaJoin, "PlazaJoin", func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().Write16(lobbyID).Write16(2).Msg())
})

var _ = register(CMD_PlazaStatus, "PlazaStatus", func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().Write16(lobbyID).Write16(2).Msg()) // 1: Close 2: Open
})

var _ = register(CMD_PlazaExplain, "PlazaExplain", func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().Write16(lobbyID).WriteString(fmt.Sprintf("LobbyExplain %d", lobbyID)).Msg())
})
