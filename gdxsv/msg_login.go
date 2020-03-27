package main

import (
	"fmt"

	"github.com/golang/glog"
)

const (
	// 00365a70 recv_three_data
	CMD_LineCheck      CmdID = 0x6001
	CMD_Logout         CmdID = 0x6002
	CMD_ShutDown       CmdID = 0x6003
	CMD_VSUserLost     CmdID = 0x6004
	CMD_Mail           CmdID = 0x6705
	CMD_ManagerMessage CmdID = 0x6706

	// 0035b5e0 : lobby_act_00_01
	CMD_LoginType       CmdID = 0x6110 // Key
	CMD_ConnectionId    CmdID = 0x6101
	CMD_AskConnectionId CmdID = 0x6102
	CMD_WarningMessage  CmdID = 0x6103 // Key

	// 0035b8f0 : lobby_act_00_02
	CMD_RegulationHeader CmdID = 0x6820
	CMD_RegulationText   CmdID = 0x6821
	CMD_RegulationFooter CmdID = 0x6822

	// 0035bd10 : lobby_act_00_03
	// User Personal Info (Pending)

	// 0035bf80 : lobby_act_00_04
	// 0035c230 : lobby_act_00_05
	// 0035c400 : lobby_act_00_06
	// Select/Register User ID
	CMD_UserHandle CmdID = 0x6111
	CMD_UserRegist CmdID = 0x6112

	// 0035c5cc : lobby_act_00_07
	CMD_AddProgress     CmdID = 0x6118
	CMD_AskBattleResult CmdID = 0x6120
	CMD_AskGameVersion  CmdID = 0x6117
	CMD_AskGameCode     CmdID = 0x6116
	CMD_AskCountryCode  CmdID = 0x6115
	CMD_AskPlatformCode CmdID = 0x6114

	// 0035c750 : lobby_act_00_08
	CMD_AskKDDICharges    CmdID = 0x6142
	CMD_PostGameParameter CmdID = 0x6143
	CMD_WinLose           CmdID = 0x6145
	CMD_RankRanking       CmdID = 0x6144
	CMD_DeviceData        CmdID = 0x6148
	CMD_ServerMoney       CmdID = 0x6149

	CMD_AskNewsTag    CmdID = 0x6801
	CMD_NewsText      CmdID = 0x6802
	CMD_InvitationTag CmdID = 0x6810

	CMD_TopRankingTag CmdID = 0x6851

	CMD_AskPatchData         CmdID = 0x6861 // 003691b0 Send_Req_PatchData
	CMD_PatchHeader          CmdID = 0x6862
	CMD_PatchData_6863       CmdID = 0x6863
	CMD_CalcDownloadChecksum CmdID = 0x6864
	CMD_PatchPing            CmdID = 0x6865

	// lobby_act_00_09
	CMD_StartLobby CmdID = 0x6141

	// lobby_sub_01_03
	CMD_PlazaMax     CmdID = 0x6203
	CMD_PlazaTitle   CmdID = 0x6204 // UNUSED
	CMD_PlazaJoin    CmdID = 0x6205
	CMD_PlazaStatus  CmdID = 0x6206
	CMD_PlazaExplain CmdID = 0x620a

	// lobby_sub_04_10
	CMD_PostChatMessage CmdID = 0x6701
	CMD_ChatMessage     CmdID = 0x6702
	CMD_LobbyRemove     CmdID = 0x64C0
	CMD_RoomTitle       CmdID = 0x6402
	CMD_RoomStatus      CmdID = 0x6404
)

func RequestLineCheck(p *AppPeer) {
	p.SendMessage(NewServerQuestion(CMD_LineCheck))
}

var _ = register(CMD_LineCheck, "LineCheck", func(p *AppPeer, m *Message) {
	// the client is alive
})

var _ = register(CMD_Logout, "Logout", func(p *AppPeer, m *Message) {
	// the client is logging out
})

func SendServerShutDown(p *AppPeer) {
	// FIXME: doesnt work
	n := NewServerNotice(CMD_ShutDown)
	w := n.Writer()
	w.WriteString("<BODY><LF=6><CENTER>サーバがシャットダウンしました<END>")
	p.SendMessage(n)
}

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

var _ = register(CMD_PostGameParameter, "CMD_PostGameParameter", func(p *AppPeer, m *Message) {
	// Client sends length-prefixed 640 bytes binary data.
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_AskKDDICharges, "CMD_AskKDDICharges", func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().Write32(123).Msg())
})

var _ = register(CMD_AskNewsTag, "CMD_AskNewsTag", func(p *AppPeer, m *Message) {
	a := NewServerAnswer(m)
	w := a.Writer()
	w.Write8(0)               // news count
	w.WriteString("News Tag") // news_tag
	p.SendMessage(a)
})

var _ = register(CMD_AskPatchData, "CMD_AskPatchData_6861", func(p *AppPeer, m *Message) {
	r := m.Reader()
	platform := r.Read8()
	crule := r.Read8()
	data := r.ReadString()
	_ = platform
	_ = crule
	_ = data

	glog.Infoln(platform, crule, data)

	a := NewServerAnswer(m)
	a.Status = StatusError // this means no patch data probably.
	p.SendMessage(a)
})

var _ = register(CMD_RankRanking, "Ranking", func(p *AppPeer, m *Message) {
	nowTopRank := m.Reader().Read8()
	_ = nowTopRank

	userRank2 := uint8(111)
	userRanking2 := uint32(222)
	userRankingTotal2 := uint32(333)
	p.SendMessage(NewServerAnswer(m).Writer().
		Write8(userRank2).
		Write32(userRanking2).
		Write32(userRankingTotal2).Msg())
})

var _ = register(CMD_WinLose, "WinLose", func(p *AppPeer, m *Message) {
	nowTopRank := m.Reader().Read8()
	_ = nowTopRank

	userBattle := uint16(1001)
	userWin := uint16(1002)
	userLose := uint16(1003)
	userDraw := uint16(1004)
	userInvalid := uint16(1005)
	userBattlePoint1 := uint32(1006)
	userBattlePoint2 := uint32(1007)
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(userBattle).
		Write16(userWin).
		Write16(userLose).
		Write16(userDraw).
		Write16(userInvalid).
		Write32(userBattlePoint1).
		Write32(userBattlePoint2).Msg())
})

var _ = register(CMD_DeviceData, "DeviceData", func(p *AppPeer, m *Message) {
	r := m.Reader()
	// Read16 * 8
	r.Read16()
	r.Read16()
	r.Read16()
	r.Read16()
	r.Read16()
	r.Read16()
	r.Read16()
	r.Read16()

	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_TopRankingTag, "TopRankingTag", func(p *AppPeer, m *Message) {
	topRankSuu := uint8(1)
	topRankTag := "top rank"
	p.SendMessage(NewServerAnswer(m).Writer().
		Write8(topRankSuu).
		WriteString(topRankTag).Msg())
})

var _ = register(CMD_ServerMoney, "ServerMoney", func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(1).
		Write16(2).
		Write16(3).
		Write16(4).Msg())
})

var _ = register(CMD_StartLobby, "StartLobby", func(p *AppPeer, m *Message) {
	// TODO: find recv func
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_InvitationTag, "InvitationTag", func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().
		WriteString("tabbuf").
		WriteString("invitation").
		Write8(0).Msg())
})

var _ = register(CMD_PlazaMax, "PlazaMax", func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(1).Msg())
})

/*
var _ = register(CMD_PlazaTitle, "PlazaTitle", func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().Write16(lobbyID).WriteString(fmt.Sprint("LobbyTitle", lobbyID)).Msg())
})
*/

var _ = register(CMD_PlazaJoin, "PlazaJoin", func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(lobbyID).
		Write16(lobbyID & 0xFF).Msg())
})

var _ = register(CMD_PlazaStatus, "PlazaStatus", func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(lobbyID).
		Write8(0xFF).Msg())
})

var _ = register(CMD_PlazaExplain, "PlazaExplain", func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(lobbyID).
		WriteString(fmt.Sprintf("<BODY>LobbyExplain %d<END>", lobbyID)).Msg())
})

var _ = register(CMD_PostChatMessage, "PostChatMessage", func(p *AppPeer, m *Message) {
	msg := m.Reader().ReadShiftJISString()
	n := NewServerNotice(CMD_ChatMessage)
	w := n.Writer()
	w.WriteString("USERID")
	w.WriteString("HANDLE_NAME")
	w.WriteString(msg)
	w.Write8(0) // chat_type
	w.Write8(0) // id color
	w.Write8(0) // handle color
	w.Write8(0) // msg color
	p.SendMessage(n)
})
