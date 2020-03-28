package main

import (
	"fmt"

	"github.com/golang/glog"
)

type MessageHandler func(*AppPeer, *Message)

var defaultHandlers = map[CmdID]MessageHandler{}

func register(id CmdID, f MessageHandler) interface{} {
	defaultHandlers[id] = f
	return nil
}

// ===========================================
//          Lobby Server Commands
// ===========================================
// To find out sending place in the game:
//   1. Run the game on pcsx2@gdxsv-dev
//   2. Find 'SetSendCommand' trace in pcsx2 debug log.
//   3. Open ps2dis and jump to the 'ra' address.
// To find out reciving place in the game:
//   1. Open ps2dis and find symbol starts with 'Acc_XXX'
//
// trace sample:
// === dump_state ===
// pc: 00365dc0 SetSendCommand
// a0: 00aa64f0 z_un_004a307c
// a1: 00006203 (00006203)
// a2: 00aa61e0 z_un_004a307c
// a3: 00000300 (00000300)
// ra: 00367718 Send_Req_PlazaMax
// >> trace
//  0: 00365dc0 SetSendCommand (+0h)
//  1: 00367718 Send_Req_PlazaMax (+6h)
//  2: 00375f30 network_connect (+108h)
//  3: 00375544 dcon_task (+45h)
//  4: 001e2c38 net_main (+6h)
//  5: 0015d6f4 N_main_loop (+17h)
//  6: 0015d390 N_main_loop (+68h)
//  7: 001000c0 (0008fe40) (+114848h)
//  8: 00000000 (ffffffff) (+0h)

const (
	CMD_LineCheck      CmdID = 0x6001
	CMD_Logout         CmdID = 0x6002
	CMD_ShutDown       CmdID = 0x6003
	CMD_VSUserLost     CmdID = 0x6004
	CMD_SendMail       CmdID = 0x6704
	CMD_Mail           CmdID = 0x6705
	CMD_ManagerMessage CmdID = 0x6706

	CMD_LoginType            CmdID = 0x6110
	CMD_ConnectionId         CmdID = 0x6101
	CMD_AskConnectionId      CmdID = 0x6102
	CMD_WarningMessage       CmdID = 0x6103
	CMD_RegulationHeader     CmdID = 0x6820
	CMD_RegulationText       CmdID = 0x6821
	CMD_RegulationFooter     CmdID = 0x6822
	CMD_UserHandle           CmdID = 0x6111
	CMD_UserRegist           CmdID = 0x6112
	CMD_AddProgress          CmdID = 0x6118
	CMD_AskBattleResult      CmdID = 0x6120
	CMD_AskGameVersion       CmdID = 0x6117
	CMD_AskGameCode          CmdID = 0x6116
	CMD_AskCountryCode       CmdID = 0x6115
	CMD_AskPlatformCode      CmdID = 0x6114
	CMD_AskKDDICharges       CmdID = 0x6142
	CMD_PostGameParameter    CmdID = 0x6143
	CMD_WinLose              CmdID = 0x6145
	CMD_RankRanking          CmdID = 0x6144
	CMD_DeviceData           CmdID = 0x6148
	CMD_ServerMoney          CmdID = 0x6149
	CMD_AskNewsTag           CmdID = 0x6801
	CMD_NewsText             CmdID = 0x6802
	CMD_InvitationTag        CmdID = 0x6810
	CMD_TopRankingTag        CmdID = 0x6851
	CMD_TopRankingSuu        CmdID = 0x6852
	CMD_TopRanking           CmdID = 0x6853
	CMD_AskPatchData         CmdID = 0x6861
	CMD_PatchHeader          CmdID = 0x6862
	CMD_PatchData_6863       CmdID = 0x6863
	CMD_CalcDownloadChecksum CmdID = 0x6864
	CMD_PatchPing            CmdID = 0x6865

	CMD_StartLobby         CmdID = 0x6141
	CMD_PlazaMax           CmdID = 0x6203
	CMD_PlazaTitle         CmdID = 0x6204 // UNUSED?
	CMD_PlazaJoin          CmdID = 0x6205
	CMD_PlazaStatus        CmdID = 0x6206
	CMD_PlazaExplain       CmdID = 0x620a
	CMD_PlazaEntry         CmdID = 0x6207
	CMD_PlazaExit          CmdID = 0x6306
	CMD_LobbyJoin          CmdID = 0x6303
	CMD_LobbyEntry         CmdID = 0x6305
	CMD_LobbyMatchingJoin  CmdID = 0x640F
	CMD_LobbyExit          CmdID = 0x6408
	CMD_RoomMax            CmdID = 0x6401
	CMD_RoomTitle          CmdID = 0x6402
	CMD_RoomStatus         CmdID = 0x6404
	CMD_RoomCreate         CmdID = 0x6407
	CMD_PutRoomName        CmdID = 0x6609
	CMD_EndRoomCreate      CmdID = 0x660C
	CMD_RoomEntry          CmdID = 0x6406
	CMD_RoomExit           CmdID = 0x6501
	CMD_RoomRemove         CmdID = 0x6505
	CMD_PostChatMessage    CmdID = 0x6701
	CMD_ChatMessage        CmdID = 0x6702
	CMD_UserSite           CmdID = 0x6703
	CMD_LobbyRemove        CmdID = 0x64C0
	CMD_LobbyMatchingEntry CmdID = 0x640E
	CMD_WaitJoin           CmdID = 0x6506
	CMD_MatchingEntry      CmdID = 0x6504
	CMD_GoToTop            CmdID = 0x6208

	CMD_ReadyBattle     CmdID = 0x6910
	CMD_AskMatchingJoin CmdID = 0x6911
	CMD_AskPlayerSide   CmdID = 0x6912
	CMD_AskPlayerInfo   CmdID = 0x6913
	CMD_AskRuleData     CmdID = 0x6914
	CMD_AskBattleCode   CmdID = 0x6915
	CMD_AskMcsAddress   CmdID = 0x6916
	CMD_AskMcsVersion   CmdID = 0x6917
	CMD_MatchingCancel  CmdID = 0x6005
)

func RequestLineCheck(p *AppPeer) {
	p.SendMessage(NewServerQuestion(CMD_LineCheck))
}

var _ = register(CMD_LineCheck, func(p *AppPeer, m *Message) {
	// the client is alive
})

var _ = register(CMD_Logout, func(p *AppPeer, m *Message) {
	// the client is logging out
})

func SendServerShutDown(p *AppPeer) {
	// FIXME: doesnt work
	n := NewServerNotice(CMD_ShutDown)
	w := n.Writer()
	w.WriteString("<BODY>サーバがシャットダウンしました<END>")
	p.SendMessage(n)
	glog.Infoln("Sending ShutDown")
}

func StartLoginFlow(p *AppPeer) {
	p.SendMessage(NewServerQuestion(CMD_AskConnectionId))
}

var _ = register(CMD_AskConnectionId, func(p *AppPeer, m *Message) {
	connID := m.Reader().ReadString()

	// We use initial connID to identify a user.
	// The value should be written by patch.
	if len(connID) != 8 {
		glog.Warning("invalid connection id: ", connID)
		p.OnClose()
		return
	}

	glog.Info("connID", connID)
	account, err := getDB().GetAccountBySessionID(connID)
	if err != nil {
		// We use initial connID as loginKey
		loginKey := connID
		account, err = getDB().GetAccountByLoginKey(loginKey)
		if err != nil {
			glog.Info("register account")
			account, err = getDB().RegisterAccountWithLoginKey(p.conn.Address(), loginKey)
			if err != nil {
				glog.Error("failed to create account", err)
				p.OnClose()
				return
			}
		}
	}

	getDB().LoginAccount(account)
	p.SessionID = account.SessionID // generated session id

	// TODO: should use temporary connection id
	p.SendMessage(NewServerQuestion(CMD_ConnectionId).Writer().
		WriteString(p.SessionID).Msg())
})

var _ = register(CMD_ConnectionId, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerNotice(CMD_WarningMessage).Writer().
		Write8(0).Msg())
})

var _ = register(CMD_RegulationHeader, func(p *AppPeer, m *Message) {
	glog.Infoln("RegurationHeader")
	p.SendMessage(NewServerAnswer(m).Writer().
		WriteString("1000").
		WriteString("1000").Msg())
	p.SendMessage(NewServerNotice(CMD_RegulationText).Writer().
		WriteString("tag").
		WriteString("text").Msg())
	p.SendMessage(NewServerNotice(CMD_RegulationFooter))
	p.SendMessage(NewServerQuestion(CMD_LoginType))
})

var _ = register(CMD_LoginType, func(p *AppPeer, m *Message) {
	loginType := m.Reader().Read8()

	// loginType == 0 means the user have an account.
	if loginType == 0 {
		account, err := getDB().GetAccountBySessionID(p.SessionID)
		if err != nil {
			glog.Warning("failed to account : ", p.SessionID)
			p.OnClose()
			return
		}

		users, err := getDB().GetUserList(account.LoginKey)
		if err != nil {
			glog.Warning("failed to get user list", account.SessionID)
			p.OnClose()
			return
		}

		n := NewServerNotice(CMD_UserHandle)
		w := n.Writer()
		w.Write8(uint8(len(users)))
		for _, u := range users {
			w.WriteString(u.UserID)
			w.WriteString(u.Name)
		}
		p.SendMessage(n)
	} else {
		// The original user registration flow uses real personal information.
		// We don't implement this because we don't want to collect personal information.
		glog.Warning("UNSUPPORTED LOGIN TYPE", loginType)
		p.OnClose()
	}
})

var _ = register(CMD_UserRegist, func(p *AppPeer, m *Message) {
	r := m.Reader()
	userID := r.ReadString() // ******
	handleName := r.ReadShiftJISString()
	glog.Infoln("UserRegist", userID, handleName)

	account, err := getDB().GetAccountBySessionID(p.SessionID)
	if err != nil {
		glog.Errorln("failed to get account :", err, p.SessionID)
		p.OnClose()
		return
	}

	if userID == "******" {
		// The peer wants to create new user.
		glog.Info("register new user :", err, account.SessionID)
		u, err := getDB().RegisterUser(account.LoginKey)
		if err != nil {
			glog.Errorln("failed to register user :", err, account.SessionID)
			p.OnClose()
			return
		}
		userID = u.UserID
	}

	u, err := getDB().GetUser(userID)
	if err != nil {
		glog.Errorln("failed to get user :", err, userID)
		p.OnClose()
		return
	}

	u.SessionID = p.SessionID
	err = getDB().LoginUser(u)
	if err != nil {
		glog.Errorln("failed to login user :", err, userID)
		p.OnClose()
		return
	}

	p.SendMessage(NewServerAnswer(m).Writer().WriteString(userID).Msg())
})

var _ = register(0x6113, func(p *AppPeer, m *Message) {
	userID := m.Reader().ReadString()
	glog.Infoln("DecideUserId", userID)
	p.SendMessage(NewServerAnswer(m).Writer().WriteString(userID).Msg())

	p.SendMessage(NewServerNotice(CMD_AddProgress)) // right?
})

var _ = register(CMD_PostGameParameter, func(p *AppPeer, m *Message) {
	// Client sends length-prefixed 640 bytes binary data.
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_AskKDDICharges, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().Write32(123).Msg())
})

var _ = register(CMD_AskNewsTag, func(p *AppPeer, m *Message) {
	a := NewServerAnswer(m)
	w := a.Writer()
	w.Write8(0)               // news count
	w.WriteString("News Tag") // news_tag
	p.SendMessage(a)
})

var _ = register(CMD_AskPatchData, func(p *AppPeer, m *Message) {
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

var _ = register(CMD_RankRanking, func(p *AppPeer, m *Message) {
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

var _ = register(CMD_WinLose, func(p *AppPeer, m *Message) {
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

var _ = register(CMD_DeviceData, func(p *AppPeer, m *Message) {
	r := m.Reader()
	// Read16 * 8
	data1 := r.Read16()
	data2 := r.Read16()
	data3 := r.Read16()
	data4 := r.Read16()
	data5 := r.Read16()
	data6 := r.Read16()
	data7 := r.Read16()
	data8 := r.Read16()
	glog.Info("DeviceData",
		data1, data2, data3, data4, data5, data6, data7, data8)

	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_ServerMoney, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(1).
		Write16(2).
		Write16(3).
		Write16(4).Msg())
})

var _ = register(CMD_StartLobby, func(p *AppPeer, m *Message) {
	// TODO: find recv func
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_InvitationTag, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().
		WriteString("tabbuf").
		WriteString("invitation").
		Write8(0).Msg())
})

var _ = register(CMD_PlazaMax, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(1).Msg())
})

/*
var _ = register(CMD_PlazaTitle, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().Write16(lobbyID).WriteString(fmt.Sprint("LobbyTitle", lobbyID)).Msg())
})
*/

var _ = register(CMD_PlazaJoin, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(lobbyID).
		Write16(lobbyID & 0xFF).Msg())
})

var _ = register(CMD_PlazaStatus, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(lobbyID).
		Write8(0xFF).Msg())
})

var _ = register(CMD_PlazaExplain, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(lobbyID).
		WriteString(fmt.Sprintf("<BODY>LobbyExplain %d<END>", lobbyID)).Msg())
})

var _ = register(CMD_PlazaEntry, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	_ = lobbyID
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_PlazaExit, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_LobbyJoin, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	_ = lobbyID
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(1).
		Write16(111).Msg())
})

var _ = register(CMD_LobbyEntry, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	_ = lobbyID
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_LobbyMatchingJoin, func(p *AppPeer, m *Message) {
	side := m.Reader().Read16()
	_ = side
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(side).
		Write16(10 + side).Msg())
})

var _ = register(CMD_RoomStatus, func(p *AppPeer, m *Message) {
	roomID := m.Reader().Read16() // ?
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(roomID).
		Write8(1).Msg())
})

var _ = register(CMD_PostChatMessage, func(p *AppPeer, m *Message) {
	msg := m.Reader().ReadShiftJISString()
	p.SendMessage(NewServerNotice(CMD_ChatMessage).Writer().
		WriteString("USERID").
		WriteString("HANDLE_NAME").
		WriteString(msg).
		Write8(0).       // chat_type
		Write8(0).       // id color
		Write8(0).       // handle color
		Write8(0).Msg()) // msg color
})

var _ = register(CMD_LobbyExit, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_RoomMax, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(10).Msg())
})

var _ = register(CMD_RoomTitle, func(p *AppPeer, m *Message) {
	roomID := m.Reader().Read16()
	title := "(RoomTitle)"
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(roomID).WriteString(title).Msg())
})

var _ = register(CMD_RoomStatus, func(p *AppPeer, m *Message) {
	roomID := m.Reader().Read16()
	status := uint8(roomID)
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(roomID).Write8(status).Msg())
})

var _ = register(CMD_RoomCreate, func(p *AppPeer, m *Message) {
	roomID := m.Reader().Read16()
	_ = roomID
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(0).
		Write16(1).
		Write16(2).
		Write8(3).
		Write8(4).
		Write8(5).
		WriteString("usersite").Msg())
})

var _ = register(CMD_PutRoomName, func(p *AppPeer, m *Message) {
	roomName := m.Reader().ReadShiftJISString()
	_ = roomName
	glog.Infoln("roomname", roomName)
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_EndRoomCreate, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_LobbyMatchingEntry, func(p *AppPeer, m *Message) {
	side := m.Reader().Read8()
	_ = side
	p.SendMessage(NewServerAnswer(m))

	// Debug
	NotifyReadyBattle(p)
})

var _ = register(CMD_SendMail, func(p *AppPeer, m *Message) {
	r := m.Reader()
	userID := r.ReadString()
	comment1 := r.ReadShiftJISString()
	comment2 := r.ReadShiftJISString()
	glog.Infoln("UserID", userID)
	glog.Infoln("com1", comment1)
	glog.Infoln("com2", comment2)
	p.SendMessage(NewServerAnswer(m)) // TODO: find reading place
})

var _ = register(CMD_UserSite, func(p *AppPeer, m *Message) {
	userID := m.Reader().ReadString()
	_ = userID
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(0).
		Write16(1).
		Write16(2).
		Write8(3).
		Write8(4).
		Write8(5).
		WriteString("<BODY>usersite<END>").Msg())
})

var _ = register(CMD_WaitJoin, func(p *AppPeer, m *Message) {
	unk := uint16(1)
	p.SendMessage(NewServerAnswer(m).Writer().Write16(unk).Msg())
})

var _ = register(CMD_RoomExit, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m))
	// RoomLeaver
})

var _ = register(CMD_RoomEntry, func(p *AppPeer, m *Message) {
	r := m.Reader()
	roomID := r.Read16()
	unk := r.Read16()
	glog.Infoln("room entry", roomID, unk)

	p.SendMessage(NewServerAnswer(m))
})

var _ = register(CMD_MatchingEntry, func(p *AppPeer, m *Message) {
	entry := m.Reader().Read8()
	if entry == 1 {
		p.SendMessage(NewServerAnswer(m))
		glog.Infoln("MatchingEntry")
	} else {
		glog.Infoln("MatchingEntryCancel")
		// FIXME: workaround
		// Only reply this request, client does not leave the room,
		// so notify RoomRemove command to drive out.
		a := NewServerAnswer(m)
		a.Status = StatusError
		p.SendMessage(a)
		p.SendMessage(NewServerNotice(CMD_RoomRemove).Writer().
			WriteString("").Msg())
	}
})

var _ = register(CMD_TopRankingTag, func(p *AppPeer, m *Message) {
	topRankSuu := uint8(1)
	topRankTag := "<BODY>RankingTitle<END>"
	p.SendMessage(NewServerAnswer(m).Writer().
		Write8(topRankSuu).
		WriteString(topRankTag).Msg())
})

var _ = register(CMD_TopRankingSuu, func(p *AppPeer, m *Message) {
	page := m.Reader().Read8()
	glog.Infoln("page", page)
	topRunkSuu := uint16(20)
	p.SendMessage(NewServerAnswer(m).Writer().Write16(topRunkSuu).Msg())
})

var _ = register(CMD_TopRanking, func(p *AppPeer, m *Message) {
	r := m.Reader()
	num1 := r.Read8()
	num2 := r.Read16()
	num3 := r.Read16()
	glog.Infoln("TopRanking", num1, num2, num3)

	topRankerNum := uint16(2)
	topRankStr := "<BODY>hoge<END>"
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(topRankerNum).
		WriteString(topRankStr).Msg())
})

var _ = register(CMD_GoToTop, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m))
})

func NotifyReadyBattle(p *AppPeer) {
	p.SendMessage(NewServerNotice(CMD_ReadyBattle))
}

var _ = register(CMD_AskMatchingJoin, func(p *AppPeer, m *Message) {
	// how many players in the game
	p.SendMessage(NewServerAnswer(m).Writer().Write8(1).Msg())
})

var _ = register(CMD_AskPlayerSide, func(p *AppPeer, m *Message) {
	_ = m.Reader().Read8() // always 1
	p.SendMessage(NewServerAnswer(m).Writer().Write8(1).Msg())
})

var _ = register(CMD_AskPlayerInfo, func(p *AppPeer, m *Message) {
	pos := m.Reader().Read8()
	p.SendMessage(NewServerAnswer(m).Writer().
		Write8(pos).
		WriteString("USERID").
		WriteString("部隊名").
		WriteString("パイロット名").
		Write16(1).
		Write16(1).
		Write16(1).
		Write16(1).
		Write16(1).
		Write16(1).
		Write16(1).
		Write16(1).Msg())
})

var _ = register(CMD_AskRuleData, func(p *AppPeer, m *Message) {
	// Binary rule data
	// TODO: investigate the format.
	// 001e2980: NetRecvHeyaBinDef default values
	// 001e2830: NetHeyaDataSet    overwrite ?
	p.SendMessage(NewServerAnswer(m).Writer().
		Write32(0x0000).
		Msg())
})

var _ = register(CMD_AskBattleCode, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().WriteString("12345").Msg())
})

var _ = register(CMD_AskMcsAddress, func(p *AppPeer, m *Message) {
	mcsAddr1 := "0011"
	mcsAddr2 := "0022"
	p.SendMessage(NewServerAnswer(m).Writer().
		WriteString(mcsAddr1).
		WriteString(mcsAddr2).Msg())
})

var _ = register(CMD_AskMcsVersion, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().Write8(10).Msg())

	// 00557fe0 sw_set_jump_tbl
	// ReflectMsg
})
