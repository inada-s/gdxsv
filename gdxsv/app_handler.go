package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
	"golang.org/x/text/width"

	"gdxsv/gdxsv/battle"
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
	lbsLineCheck      CmdID = 0x6001
	lbsLogout         CmdID = 0x6002
	lbsShutDown       CmdID = 0x6003
	lbsVSUserLost     CmdID = 0x6004
	lbsSendMail       CmdID = 0x6704
	lbsRecvMail       CmdID = 0x6705
	lbsManagerMessage CmdID = 0x6706

	lbsLoginType            CmdID = 0x6110
	lbsConnectionID         CmdID = 0x6101
	lbsAskConnectionID      CmdID = 0x6102
	lbsWarningMessage       CmdID = 0x6103
	lbsRegulationHeader     CmdID = 0x6820
	lbsRegulationText       CmdID = 0x6821
	lbsRegulationFooter     CmdID = 0x6822
	lbsUserHandle           CmdID = 0x6111
	lbsUserRegist           CmdID = 0x6112
	lbsUserDecide           CmdID = 0x6113
	lbsAddProgress          CmdID = 0x6118
	lbsAskBattleResult      CmdID = 0x6120
	lbsAskGameVersion       CmdID = 0x6117
	lbsAskGameCode          CmdID = 0x6116
	lbsAskCountryCode       CmdID = 0x6115
	lbsAskPlatformCode      CmdID = 0x6114
	lbsAskKDDICharges       CmdID = 0x6142
	lbsPostGameParameter    CmdID = 0x6143
	lbsWinLose              CmdID = 0x6145
	lbsRankRanking          CmdID = 0x6144
	lbsDeviceData           CmdID = 0x6148
	lbsServerMoney          CmdID = 0x6149
	lbsAskNewsTag           CmdID = 0x6801
	lbsNewsText             CmdID = 0x6802
	lbsInvitationTag        CmdID = 0x6810
	lbsTopRankingTag        CmdID = 0x6851
	lbsTopRankingSuu        CmdID = 0x6852
	lbsTopRanking           CmdID = 0x6853
	lbsAskPatchData         CmdID = 0x6861
	lbsPatchHeader          CmdID = 0x6862
	lbsPatchData6863        CmdID = 0x6863
	lbsCalcDownloadChecksum CmdID = 0x6864
	lbsPatchPing            CmdID = 0x6865

	lbsStartLobby         CmdID = 0x6141
	lbsPlazaMax           CmdID = 0x6203
	lbsPlazaTitle         CmdID = 0x6204 // UNUSED?
	lbsPlazaJoin          CmdID = 0x6205
	lbsPlazaStatus        CmdID = 0x6206
	lbsPlazaExplain       CmdID = 0x620a
	lbsPlazaEntry         CmdID = 0x6207 // Select a lobby
	lbsPlazaExit          CmdID = 0x6306 // Exit a lobby
	lbsLobbyJoin          CmdID = 0x6303 //
	lbsLobbyEntry         CmdID = 0x6305 // Select join side and enter lobby chat scene
	lbsLobbyExit          CmdID = 0x6408 // Exit lobby chat and enter join side select scene
	lbsLobbyMatchingJoin  CmdID = 0x640F
	lbsRoomMax            CmdID = 0x6401
	lbsRoomTitle          CmdID = 0x6402
	lbsRoomStatus         CmdID = 0x6404
	lbsRoomCreate         CmdID = 0x6407
	lbsPutRoomName        CmdID = 0x6609
	lbsEndRoomCreate      CmdID = 0x660C
	lbsRoomEntry          CmdID = 0x6406
	lbsRoomExit           CmdID = 0x6501
	lbsRoomLeaver         CmdID = 0x6502
	lbsRoomCommer         CmdID = 0x6503
	lbsRoomRemove         CmdID = 0x6505
	lbsPostChatMessage    CmdID = 0x6701
	lbsChatMessage        CmdID = 0x6702
	lbsUserSite           CmdID = 0x6703
	lbsLobbyRemove        CmdID = 0x64C0
	lbsLobbyMatchingEntry CmdID = 0x640E
	lbsWaitJoin           CmdID = 0x6506
	lbsMatchingEntry      CmdID = 0x6504 // Room matching
	lbsGoToTop            CmdID = 0x6208

	lbsReadyBattle     CmdID = 0x6910
	lbsAskMatchingJoin CmdID = 0x6911
	lbsAskPlayerSide   CmdID = 0x6912
	lbsAskPlayerInfo   CmdID = 0x6913
	lbsAskRuleData     CmdID = 0x6914
	lbsAskBattleCode   CmdID = 0x6915
	lbsAskMcsAddress   CmdID = 0x6916
	lbsAskMcsVersion   CmdID = 0x6917
	lbsMatchingCancel  CmdID = 0x6005
)

func RequestLineCheck(p *AppPeer) {
	p.SendMessage(NewServerQuestion(lbsLineCheck))
}

var _ = register(lbsLineCheck, func(p *AppPeer, m *Message) {
	// the client is alive
})

var _ = register(lbsLogout, func(p *AppPeer, m *Message) {
	// the client is logging out
	if p.Room != nil {
		p.Room.Exit(p.UserID)
		p.app.BroadcastRoomState(p.Room)
		p.Room = nil
	}
	if p.Lobby != nil {
		p.Lobby.Exit(p.UserID)
		p.app.BroadcastLobbyUserCount(p.Lobby.ID)
		p.Lobby = nil
	}
})

func SendServerShutDown(p *AppPeer) {
	n := NewServerNotice(lbsShutDown)
	w := n.Writer()
	w.WriteString("<LF=6><BODY><CENTER>サーバがシャットダウンしました<END>")
	p.SendMessage(n)
	glog.Infoln("Sending ShutDown")
}

func StartLoginFlow(p *AppPeer) {
	m := NewServerQuestion(lbsAskConnectionID)
	glog.Infoln("lbsAskConnectionID:", hex.EncodeToString(m.Serialize()))
	p.SendMessage(NewServerQuestion(lbsAskConnectionID))
}

var _ = register(lbsAskConnectionID, func(p *AppPeer, m *Message) {
	connID := m.Reader().ReadString()

	// We use initial connID to identify a user.
	// The value should be written by patch.
	if len(connID) != 8 {
		glog.Warning("invalid connection id: ", connID)
		p.conn.Close()
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
			account, err = getDB().RegisterAccountWithLoginKey(p.Address(), loginKey)
			if err != nil {
				glog.Error("failed to create account", err)
				p.conn.Close()
				return
			}
		}
	}

	getDB().LoginAccount(account)
	p.SessionID = account.SessionID // generated session id

	p.SendMessage(NewServerQuestion(lbsConnectionID).Writer().
		WriteString(p.SessionID).Msg())
})

var _ = register(lbsConnectionID, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerNotice(lbsWarningMessage).Writer().
		Write8(0).Msg())
})

var _ = register(lbsRegulationHeader, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().
		WriteString("1000").
		WriteString("1000").Msg())
	p.SendMessage(NewServerNotice(lbsRegulationText).Writer().
		WriteString("tag").
		WriteString("text").Msg())
	p.SendMessage(NewServerNotice(lbsRegulationFooter))
	p.SendMessage(NewServerQuestion(lbsLoginType))
})

var _ = register(lbsLoginType, func(p *AppPeer, m *Message) {
	loginType := m.Reader().Read8()

	// loginType == 0 means the user have an account.
	// loginType == 3 means the user seems to have returned from match.
	if loginType == 0 || loginType == 3 {
		account, err := getDB().GetAccountBySessionID(p.SessionID)
		if err != nil {
			glog.Warning("failed to account : ", p.SessionID)
			p.conn.Close()
			return
		}

		users, err := getDB().GetUserList(account.LoginKey)
		if err != nil {
			glog.Warning("failed to get user list", account.SessionID)
			p.conn.Close()
			return
		}

		n := NewServerNotice(lbsUserHandle)
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
		p.conn.Close()
	}
})

var _ = register(lbsUserRegist, func(p *AppPeer, m *Message) {
	r := m.Reader()
	userID := r.ReadString() // ******
	handleName := r.ReadShiftJISString()
	glog.Infoln("UserRegist", userID, handleName)

	account, err := getDB().GetAccountBySessionID(p.SessionID)
	if err != nil {
		glog.Errorln("failed to get account :", err, p.SessionID)
		p.conn.Close()
		return
	}

	if userID == "******" {
		// The peer wants to create new user.
		glog.Info("register new user :", err, account.SessionID)
		u, err := getDB().RegisterUser(account.LoginKey)
		if err != nil {
			glog.Errorln("failed to register user :", err, account.SessionID)
			p.conn.Close()
			return
		}
		userID = u.UserID
	}

	u, err := getDB().GetUser(userID)
	if err != nil {
		glog.Errorln("failed to get user :", err, userID)
		p.conn.Close()
		return
	}

	err = getDB().LoginUser(u)
	if err != nil {
		glog.Errorln("failed to login user :", err, userID)
		p.conn.Close()
		return
	}

	u.Name = handleName
	u.SessionID = p.SessionID
	err = getDB().UpdateUser(u)
	if err != nil {
		glog.Errorln("failed to save user :", err, userID)
		p.conn.Close()
		return
	}

	p.DBUser = *u
	p.app.users[p.UserID] = p
	p.SendMessage(NewServerAnswer(m).Writer().WriteString(userID).Msg())
})

var _ = register(lbsUserDecide, func(p *AppPeer, m *Message) {
	userID := m.Reader().ReadString()
	glog.Infoln("DecideUserId", userID)

	u, err := getDB().GetUser(userID)
	if err != nil {
		glog.Errorln("failed to get user :", err, userID)
		p.conn.Close()
		return
	}

	err = getDB().LoginUser(u)
	if err != nil {
		glog.Errorln("failed to login user :", err, userID)
		p.conn.Close()
		return
	}

	u.SessionID = p.SessionID
	err = getDB().UpdateUser(u)
	if err != nil {
		glog.Errorln("failed to save user :", err, userID)
		p.conn.Close()
		return
	}

	p.DBUser = *u
	p.app.users[p.UserID] = p
	p.SendMessage(NewServerAnswer(m).Writer().WriteString(p.UserID).Msg())
	p.SendMessage(NewServerQuestion(lbsAskBattleResult))
})

var _ = register(lbsAskBattleResult, func(p *AppPeer, m *Message) {
	r := m.Reader()
	unk1 := r.ReadString()
	unk2 := r.Read8()
	unk3 := r.Read8()
	unk4 := r.Read8()
	unk5 := r.Read8()
	unk6 := r.Read8()
	unk7 := r.Read8()
	unk8 := r.Read32()
	unk9 := r.Read8()
	unk10 := r.Read8()
	unk11 := r.Read8()
	unk12 := r.Read8()
	unk13 := r.Read16()
	unk14 := r.Read16()
	unk15 := r.Read16()
	unk16 := r.Read16()
	unk17 := r.Read16()
	unk18 := r.Read16()
	unk19 := r.Read16()
	unk20 := r.Read16()
	unk21 := r.Read16()
	unk22 := r.Read16()
	unk23 := r.Read16()
	unk24 := r.Read16()
	unk25 := r.Read16()
	unk26 := r.Read16()
	unk27 := r.Read16()
	unk28 := r.Read16()
	result := &BattleResult{
		unk1, unk2, unk3, unk4, unk5, unk6,
		unk7, unk8, unk9, unk10, unk11, unk12,
		unk13, unk14, unk15, unk16, unk17, unk18,
		unk19, unk20, unk21, unk22, unk23, unk24,
		unk25, unk26, unk27, unk28,
	}
	p.app.OnGetBattleResult(p, result)
	p.SendMessage(NewServerNotice(lbsAddProgress))
})

var _ = register(lbsPostGameParameter, func(p *AppPeer, m *Message) {
	// Client sends length-prefixed 640 bytes binary data.
	// This is used when goto battle scene.
	p.GameParam = m.Reader().ReadBytes()

	// The data consists of keyconfig and pilot name.
	// Pick pilot name.
	r := m.Reader()
	var buf []byte
	for i := 0; i < 18; i++ {
		r.Read8()
	}
	for 0 < r.Remaining() {
		v := r.Read8()
		if v == 0 {
			break
		}
		buf = append(buf, v)
	}
	bin, err := ioutil.ReadAll(transform.NewReader(bytes.NewReader(buf), japanese.ShiftJIS.NewDecoder()))
	if err != nil {
		glog.Errorln(err)
	}
	p.PilotName = string(bin)

	p.SendMessage(NewServerAnswer(m))
})

var _ = register(lbsAskKDDICharges, func(p *AppPeer, m *Message) {
	// 課金予測情報 (円)
	p.SendMessage(NewServerAnswer(m).Writer().Write32(0).Msg())
})

var _ = register(lbsAskNewsTag, func(p *AppPeer, m *Message) {
	a := NewServerAnswer(m)
	w := a.Writer()
	w.Write8(0)               // news count
	w.WriteString("News Tag") // news_tag
	p.SendMessage(a)
})

var _ = register(lbsAskPatchData, func(p *AppPeer, m *Message) {
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

func decideGrade(winCount, rank int) uint8 {
	// grade 14 ~ 0
	// [大将][中将][少将][大佐][中佐][少佐][大尉][中尉][少尉][曹長][軍曹][伍長][上等兵][一等兵][二等兵]

	if rank == 0 {
		// when ranking is not available.
		return 0
	}

	grade := winCount / 100

	if 14 <= grade {
		grade = 14
	}

	if 12 <= grade {
		if rank <= 5 {
			rank = 14 // 1~5 [大将]
		} else if rank <= 20 {
			rank = 13 // 6~20 [中将]
		} else if rank <= 50 {
			rank = 12 // 21~50 [少将]
		} else {
			rank = 11 // 50~ [大佐]
		}
	}

	return uint8(grade)
}

var _ = register(lbsRankRanking, func(p *AppPeer, m *Message) {
	nowTopRank := m.Reader().Read8()
	ranking, err := getDB().GetWinCountRanking(0)
	if nowTopRank == 0 && err == nil {
		maxRank := len(ranking)
		p.Rank = 0
		i := sort.Search(len(ranking), func(i int) bool { return ranking[i].WinCount <= p.WinCount })
		if i < len(ranking) && ranking[i].WinCount == p.WinCount {
			p.Rank = ranking[i].Rank
		} else {
			p.Rank = i // means out of rank
		}
		grade := decideGrade(p.WinCount, p.Rank)

		p.SendMessage(NewServerAnswer(m).Writer().
			Write8(uint8(grade)).
			Write32(uint32(p.Rank)).
			Write32(uint32(maxRank)).Msg())
	} else {
		p.SendMessage(NewServerAnswer(m).Writer().
			Write8(uint8(10)).
			Write32(uint32(20)).
			Write32(uint32(30)).Msg())
	}
})

var _ = register(lbsWinLose, func(p *AppPeer, m *Message) {
	nowTopRank := m.Reader().Read8()
	if nowTopRank == 0 {
		grade := decideGrade(p.WinCount, p.Rank)
		userWin := r16(p.WinCount)
		userLose := r16(p.LoseCount)
		userDraw := uint16(0)
		userInvalid := r16(p.BattleCount - p.WinCount - p.LoseCount)
		userBattlePoint1 := uint32(0)
		userBattlePoint2 := uint32(0)

		p.SendMessage(NewServerAnswer(m).Writer().
			Write16(uint16(grade)).
			Write16(userWin).
			Write16(userLose).
			Write16(userDraw).
			Write16(userInvalid).
			Write32(userBattlePoint1).
			Write32(userBattlePoint2).Msg())
	} else {
		p.SendMessage(NewServerAnswer(m).Writer().
			Write16(uint16(1)).
			Write16(100).
			Write16(100).
			Write16(100).
			Write16(0).
			Write32(1).
			Write32(1).Msg())
	}

})

var _ = register(lbsDeviceData, func(p *AppPeer, m *Message) {
	r := m.Reader()
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

var _ = register(lbsServerMoney, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(0).Write16(0).Write16(0).Write16(0).Msg())
})

var _ = register(lbsStartLobby, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(lbsInvitationTag, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().
		WriteString("tabbuf").
		WriteString("invitation").
		Write8(0).Msg())
})

var _ = register(lbsPlazaMax, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(maxLobbyCount).Msg())
})

/*
var _ = register(lbsPlazaTitle, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().Write16(lobbyID).WriteString(fmt.Sprint("LobbyTitle", lobbyID)).Msg())
})
*/

var _ = register(lbsPlazaJoin, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	lobby := p.app.lobbys[lobbyID]
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(lobbyID).
		Write16(uint16(len(lobby.Users))).Msg())
})

var _ = register(lbsPlazaStatus, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(lobbyID).
		Write8(1).Msg())
})

var _ = register(lbsPlazaExplain, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(lobbyID).
		WriteString(fmt.Sprintf("<BODY>Lobby %d<END>", lobbyID)).Msg())
})

var _ = register(lbsPlazaEntry, func(p *AppPeer, m *Message) {
	lobbyID := m.Reader().Read16()
	lobby := p.app.lobbys[lobbyID]
	p.Lobby = lobby
	p.Entry = EntryNone
	p.inLobbyChat = false

	lobby.Enter(p)
	p.SendMessage(NewServerAnswer(m))
	p.app.BroadcastLobbyUserCount(lobbyID)
})

var _ = register(lbsPlazaExit, func(p *AppPeer, m *Message) {
	if p.Lobby != nil {
		lobbyID := p.Lobby.ID

		p.Lobby.Exit(p.UserID)
		p.Lobby = nil
		p.Entry = EntryNone
		p.inLobbyChat = false

		p.SendMessage(NewServerAnswer(m))
		p.app.BroadcastLobbyUserCount(lobbyID)
		return
	}
	p.SendMessage(NewServerAnswer(m).SetErr())
})

var _ = register(lbsLobbyEntry, func(p *AppPeer, m *Message) {
	if p.Lobby != nil {
		side := m.Reader().Read16()
		p.Entry = side
		p.inLobbyChat = true
		p.SendMessage(NewServerAnswer(m))
		p.app.BroadcastLobbyUserCount(p.Lobby.ID)
		return
	}
	p.SendMessage(NewServerAnswer(m).SetErr())
})

var _ = register(lbsLobbyExit, func(p *AppPeer, m *Message) {
	if p.Lobby != nil {
		p.Entry = EntryNone
		p.inLobbyChat = false
		p.SendMessage(NewServerAnswer(m))
		p.app.BroadcastLobbyUserCount(p.Lobby.ID)
		return
	}
	p.SendMessage(NewServerAnswer(m).SetErr())
})

var _ = register(lbsLobbyJoin, func(p *AppPeer, m *Message) {
	if p.Lobby != nil {
		side := m.Reader().Read16()
		renpo, zeon := p.Lobby.GetUserCountBySide()
		if p.inLobbyChat {
			p.SendMessage(NewServerAnswer(m).Writer().
				Write16(side).Write16(renpo + zeon).Msg())
		} else {
			if side == 1 {
				p.SendMessage(NewServerAnswer(m).Writer().
					Write16(side).Write16(renpo).Msg())
			} else {
				p.SendMessage(NewServerAnswer(m).Writer().
					Write16(side).Write16(zeon).Msg())
			}
		}
		return
	}
	p.SendMessage(NewServerAnswer(m).SetErr())
})

var _ = register(lbsLobbyMatchingJoin, func(p *AppPeer, m *Message) {
	if p.Lobby != nil {
		side := m.Reader().Read16()
		renpo, zeon := p.Lobby.GetLobbyMatchEntryUserCount()
		if side == 1 {
			p.SendMessage(NewServerAnswer(m).Writer().
				Write16(side).Write16(renpo).Msg())
		} else {
			p.SendMessage(NewServerAnswer(m).Writer().
				Write16(side).Write16(zeon).Msg())
		}
		return
	}
	p.SendMessage(NewServerAnswer(m).SetErr())
})

var _ = register(lbsLobbyMatchingEntry, func(p *AppPeer, m *Message) {
	enable := m.Reader().Read8()
	if p.Lobby != nil {
		if enable == 1 {
			p.Lobby.Entry(p)
		} else {
			p.Lobby.EntryCancel(p.UserID)
		}

		if p.Lobby.CanBattleStart() {
			b := NewBattle(p.app, p.Lobby.ID)
			b.BattleCode = GenBattleCode()
			participants := p.Lobby.PickBattleUsers()
			for _, q := range participants {
				b.Add(q)
				q.Battle = b
				battle.AddUserWhoIsGoingTobattle(
					b.BattleCode, q.UserID, q.Name, q.Entry, q.SessionID)
				getDB().AddBattleRecord(&BattleRecord{
					BattleCode: b.BattleCode,
					UserID:     q.UserID,
					UserName:   q.Name,
					PilotName:  q.PilotName,
					Players:    len(participants),
					Aggregate:  1,
				})
				NotifyReadyBattle(q)
			}
		}

		p.SendMessage(NewServerAnswer(m))
		p.app.BroadcastLobbyMatchEntryUserCount(p.Lobby.ID)

		return
	}
	p.SendMessage(NewServerAnswer(m).SetErr())

})

var _ = register(lbsRoomStatus, func(p *AppPeer, m *Message) {
	roomID := m.Reader().Read16()
	if room, ok := p.Lobby.Rooms[roomID]; ok {
		p.SendMessage(NewServerAnswer(m).Writer().
			Write16(roomID).
			Write8(room.Status).Msg())
		return
	}
	p.SendMessage(NewServerAnswer(m).SetErr())
})

var _ = register(lbsRoomMax, func(p *AppPeer, m *Message) {
	if p.Lobby != nil {
		roomCount := uint16(len(p.Lobby.Rooms))
		p.SendMessage(NewServerAnswer(m).Writer().Write16(roomCount).Msg())
		return
	}
	p.SendMessage(NewServerAnswer(m).SetErr())
})

var _ = register(lbsRoomTitle, func(p *AppPeer, m *Message) {
	roomID := m.Reader().Read16()
	if p.Lobby != nil {
		if room, ok := p.Lobby.Rooms[roomID]; ok {
			p.SendMessage(NewServerAnswer(m).Writer().
				Write16(roomID).
				WriteString(room.Name).Msg())
			return
		}
	}
	p.SendMessage(NewServerAnswer(m).SetErr())
})

var _ = register(lbsRoomCreate, func(p *AppPeer, m *Message) {
	roomID := m.Reader().Read16()
	if p.Lobby != nil {
		lobby := p.Lobby
		if room, ok := lobby.Rooms[roomID]; ok {
			if room.Status == RoomStateEmpty {
				room.Status = RoomStatePrepare
				room.Owner = p.UserID
				p.Room = room
				p.SendMessage(NewServerAnswer(m))
				p.app.BroadcastRoomState(room)
				return
			}
		}
	}
	p.SendMessage(NewServerAnswer(m).SetErr().Writer().
		WriteString("<LF=6><BODY>Failed to create room<END>").Msg())
})

var _ = register(lbsPutRoomName, func(p *AppPeer, m *Message) {
	if p.Room != nil && p.Room.Owner == p.UserID && p.Room.Status == RoomStatePrepare {
		roomName := m.Reader().ReadShiftJISString()
		p.Room.Name = roomName
		p.SendMessage(NewServerAnswer(m))
		p.app.BroadcastRoomState(p.Room)
		return
	}
	p.SendMessage(NewServerAnswer(m).SetErr())
})

var _ = register(lbsEndRoomCreate, func(p *AppPeer, m *Message) {
	if p.Room != nil && p.Room.Owner == p.UserID && p.Room.Status == RoomStatePrepare {
		p.Room.Enter(&p.DBUser)
		p.SendMessage(NewServerAnswer(m))
		p.app.BroadcastRoomState(p.Room)
		return
	}
	p.SendMessage(NewServerAnswer(m).SetErr())
})

var _ = register(lbsSendMail, func(p *AppPeer, m *Message) {
	r := m.Reader()
	userID := r.ReadString()
	comment1 := r.ReadShiftJISString()
	comment2 := r.ReadShiftJISString()
	glog.Infoln("UserID", userID)
	glog.Infoln("com1", comment1)
	glog.Infoln("com2", comment2)

	if u, ok := p.app.users[userID]; ok {
		u.SendMessage(NewServerNotice(lbsRecvMail).Writer().
			WriteString(p.UserID).
			WriteString(p.Name).
			WriteString(comment1).Msg())
		p.SendMessage(NewServerAnswer(m))
	} else {
		p.SendMessage(NewServerAnswer(m).SetErr().Writer().
			WriteString("<LF=6><BODY><CENTER>THE USER IS NOT IN LOBBY<END>").Msg())
	}
})

var _ = register(lbsUserSite, func(p *AppPeer, m *Message) {
	// TODO: Implement
	userID := m.Reader().ReadString()
	_ = userID
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(0).
		Write16(1).
		Write16(2).
		Write8(3).
		Write8(4).
		Write8(5).
		WriteString("<LF=6><BODY><CENTER>UNDER CONSTRUCTION<END>").Msg())
})

var _ = register(lbsWaitJoin, func(p *AppPeer, m *Message) {
	unk := uint16(1)
	p.SendMessage(NewServerAnswer(m).Writer().Write16(unk).Msg())
})

var _ = register(lbsRoomEntry, func(p *AppPeer, m *Message) {
	r := m.Reader()
	roomID := r.Read16()
	unknown := r.Read16()

	glog.Infoln("room entry", roomID, unknown)

	if p.Lobby != nil {
		if room, ok := p.Lobby.Rooms[roomID]; ok {
			if room.Status == RoomStateRecruiting {
				room.Enter(&p.DBUser)
				p.Room = room
				for _, u := range room.Users {
					if q, ok := p.app.FindPeer(u.UserID); ok {
						q.SendMessage(NewServerNotice(lbsRoomCommer).Writer().
							WriteString(u.UserID).
							WriteString(u.Name).Msg())
					}
				}
				p.SendMessage(NewServerAnswer(m))
				return
			}
		}
	}
	p.SendMessage(NewServerAnswer(m).SetErr())
})

var _ = register(lbsRoomExit, func(p *AppPeer, m *Message) {
	defer p.SendMessage(NewServerAnswer(m))

	if p.Room == nil {
		return
	}

	r := p.Room
	p.Room = nil

	if r.Owner == p.UserID {
		for _, u := range r.Users {
			if r.Owner != u.UserID {
				if q, ok := p.app.FindPeer(u.UserID); ok {
					q.Room = nil
					q.SendMessage(NewServerNotice(lbsRoomRemove).Writer().
						WriteString("<LF=6><BODY><CENTER>部屋が解散になりました。<END>").Msg())
				}
			}
		}
		r.Remove()
	} else {
		r.Exit(p.UserID)
		for _, u := range r.Users {
			if q, ok := p.app.FindPeer(u.UserID); ok {
				q.SendMessage(NewServerNotice(lbsRoomLeaver).Writer().
					WriteString(u.UserID).
					WriteString(u.Name).Msg())
			}
		}
	}

	if p.Lobby != nil {
		p.app.BroadcastRoomState(r)
	}
})

var _ = register(lbsMatchingEntry, func(p *AppPeer, m *Message) {
	entry := m.Reader().Read8()
	if entry == 1 {
		glog.Infoln("MatchingEntry")
		p.SendMessage(NewServerAnswer(m))
	} else {
		glog.Infoln("MatchingEntryCancel")
		// FIXME: workaround
		// Only reply this request, client does not leave the room,
		// so notify RoomRemove command to drive out.
		// It doesn't work
		p.SendMessage(NewServerAnswer(m).SetErr())
		p.SendMessage(NewServerNotice(lbsRoomRemove).Writer().
			WriteString("").Msg())
	}
})

var _ = register(lbsPostChatMessage, func(p *AppPeer, m *Message) {
	text := m.Reader().ReadShiftJISString()
	msg := NewServerNotice(lbsChatMessage).Writer().
		WriteString(p.UserID).
		WriteString(p.Name).
		WriteString(text).
		Write8(0).      // chat_type
		Write8(0).      // id color
		Write8(0).      // handle color
		Write8(0).Msg() // msg color

	if p.Room != nil {
		for _, u := range p.Room.Users {
			if q, ok := p.app.FindPeer(u.UserID); ok {
				q.SendMessage(msg)
			}
		}
	} else if p.Lobby != nil {
		for userID := range p.Lobby.Users {
			if q, ok := p.app.FindPeer(userID); ok {
				q.SendMessage(msg)
			}
		}
	}
})

var _ = register(lbsTopRankingTag, func(p *AppPeer, m *Message) {
	topRankSuu := uint8(1)
	topRankTag := "勝利数ランキング"
	p.SendMessage(NewServerAnswer(m).Writer().
		Write8(topRankSuu).
		WriteString(topRankTag).Msg())
})

var _ = register(lbsTopRankingSuu, func(p *AppPeer, m *Message) {
	// How many users there is in the ranking
	// page: ranking kind?
	page := m.Reader().Read8()
	glog.Infoln("page", page)

	n := 0
	if ranking, err := getDB().GetWinCountRanking(0); err == nil {
		n = len(ranking)
	}
	p.SendMessage(NewServerAnswer(m).Writer().Write16(uint16(n)).Msg())
})

var _ = register(lbsTopRanking, func(p *AppPeer, m *Message) {
	r := m.Reader()
	num1 := r.Read8()
	num2 := r.Read16()
	num3 := r.Read16()
	glog.Infoln("TopRanking", num1, num2, num3)

	ranking, err := getDB().GetWinCountRanking(0)
	if err != nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}
	index := int(num2 - 1)

	if index < 0 || len(ranking) <= index {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	// Note: <COLOR=N>
	// 0: 白
	// 1: 赤
	// 2: 緑
	// 3: 黄
	// 4: 青
	// 5: 紫

	rec := ranking[index]
	topRankerNum := uint16(num2)
	topRankStr := fmt.Sprintf("<SIZE=4><BODY>%3d位 <COLOR=3> %s <COLOR=4>%v<BR>", rec.Rank, width.Widen.String(rec.UserID), rec.Name) +
		fmt.Sprintf("<SIZE=3><COLOR=0>%5d<COLOR=3>戦<COLOR=0> %5d<COLOR=3>勝<COLOR=0> %5d<COLOR=3>敗<COLOR=0> %5d<COLOR=3>無効<COLOR=0><END>",
			rec.BattleCount, rec.WinCount, rec.LoseCount, rec.BattleCount-rec.WinCount-rec.LoseCount)
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(topRankerNum).
		WriteString(topRankStr).Msg())
})

var _ = register(lbsGoToTop, func(p *AppPeer, m *Message) {
	p.Battle = nil

	lobbyID := uint16(0)
	roomID := uint16(0)

	if p.Room != nil {
		roomID = p.Room.ID
		p.Room.Exit(p.UserID)
		p.Room = nil
	}

	if p.Lobby != nil {
		lobbyID = p.Lobby.ID
		p.Lobby.Exit(p.UserID)
		p.Lobby = nil
	}

	p.Entry = EntryNone
	p.SendMessage(NewServerAnswer(m))

	if lobbyID != 0 {
		p.app.BroadcastLobbyUserCount(lobbyID)
		p.app.BroadcastLobbyMatchEntryUserCount(lobbyID)
		if roomID != 0 {
			// TODO broadcast about room
		}
	}
})

func NotifyReadyBattle(p *AppPeer) {
	p.SendMessage(NewServerNotice(lbsReadyBattle))
}

var _ = register(lbsAskMatchingJoin, func(p *AppPeer, m *Message) {
	// how many players in the game
	n := p.Battle.NumOfEntryUsers()
	p.SendMessage(NewServerAnswer(m).Writer().Write8(uint8(n)).Msg())
})

var _ = register(lbsAskPlayerSide, func(p *AppPeer, m *Message) {
	// player position
	p.SendMessage(NewServerAnswer(m).Writer().Write8(p.Battle.GetPosition(p.UserID)).Msg())
})

func r16(a int) uint16 {
	if math.MaxUint16 < a {
		return math.MaxUint16
	}
	return uint16(a)
}

var _ = register(lbsAskPlayerInfo, func(p *AppPeer, m *Message) {
	if p.Battle == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	pos := m.Reader().Read8()
	u := p.Battle.GetUserByPos(pos)
	param := p.Battle.GetGameParamByPos(pos)
	side := p.Battle.GetUserSide(u.UserID)
	grade := decideGrade(u.WinCount, p.Battle.GetUserRankByPos(pos))

	p.SendMessage(NewServerAnswer(m).Writer().
		Write8(pos).
		WriteString(u.UserID).
		WriteString(u.Name).
		WriteBytes(param).
		Write16(uint16(grade)).
		Write16(r16(u.WinCount)).
		Write16(r16(u.LoseCount)).
		Write16(0). // draw count
		Write16(r16(u.BattleCount - u.WinCount - u.LoseCount)).
		Write16(0). // Unknown
		Write16(side).
		Write16(0). // Unknown
		Msg())
})

var _ = register(lbsAskRuleData, func(p *AppPeer, m *Message) {
	if p.Battle == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	// Binary rule data
	// 001e2980: NetRecvHeyaBinDef default values
	// 001e2830: NetHeyaDataSet    overwrite ?
	a := NewServerAnswer(m)
	w := a.Writer()
	bin := p.Battle.Rule.Serialize()
	w.Write16(uint16(len(bin)))
	w.Write(bin)
	p.SendMessage(a)
})

var _ = register(lbsAskBattleCode, func(p *AppPeer, m *Message) {
	if p.Battle == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	p.SendMessage(NewServerAnswer(m).Writer().WriteString(p.Battle.BattleCode).Msg())
})

var _ = register(lbsAskMcsAddress, func(p *AppPeer, m *Message) {
	host, portStr, err := net.SplitHostPort(conf.BattlePublicAddr)
	if err != nil {
		glog.Errorln(err)
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		glog.Errorln(err)
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	ip, port := net.ParseIP(host), uint16(portNum)
	glog.Infoln(ip, port)
	if ip == nil || ip.To4() == nil || port == 0 {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	a := NewServerAnswer(m)
	w := a.Writer()

	bits := strings.Split(ip.String(), ".")
	b0, _ := strconv.Atoi(bits[0])
	b1, _ := strconv.Atoi(bits[1])
	b2, _ := strconv.Atoi(bits[2])
	b3, _ := strconv.Atoi(bits[3])

	w.Write16(4)
	w.Write8(byte(b0))
	w.Write8(byte(b1))
	w.Write8(byte(b2))
	w.Write8(byte(b3))
	w.Write16(4)
	w.Write16LE(port)

	p.SendMessage(a)
})

var _ = register(lbsAskMcsVersion, func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerAnswer(m).Writer().Write8(10).Msg())
})
