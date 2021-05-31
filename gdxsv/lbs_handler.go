package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"hash/fnv"
	"io/ioutil"
	"math"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
	"golang.org/x/text/width"
)

type LbsHandler func(*LbsPeer, *LbsMessage)

var defaultLbsHandlers = map[CmdID]LbsHandler{}

func register(id CmdID, f LbsHandler) interface{} {
	defaultLbsHandlers[id] = f
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

	lbsLoginType       CmdID = 0x6110
	lbsConnectionID    CmdID = 0x6101
	lbsAskConnectionID CmdID = 0x6102
	lbsWarningMessage  CmdID = 0x6103

	lbsEncodeStart CmdID = 0x613a
	lbsUserInfo1   CmdID = 0x6131
	lbsUserInfo2   CmdID = 0x6132 // UNUSED
	lbsUserInfo3   CmdID = 0x6133 // UNUSED
	lbsUserInfo4   CmdID = 0x6134 // UNUSED
	lbsUserInfo5   CmdID = 0x6135 // UNUSED
	lbsUserInfo6   CmdID = 0x6136 // UNUSED
	lbsUserInfo7   CmdID = 0x6137 // UNUSED
	lbsUserInfo8   CmdID = 0x6138 // UNUSED
	lbsUserInfo9   CmdID = 0x6139

	lbsRegulationHeader     CmdID = 0x6820
	lbsRegulationText       CmdID = 0x6821
	lbsRegulationFooter     CmdID = 0x6822
	lbsUserHandle           CmdID = 0x6111
	lbsUserRegist           CmdID = 0x6112
	lbsUserDecide           CmdID = 0x6113
	lbsAskPlatformCode      CmdID = 0x6114
	lbsAskCountryCode       CmdID = 0x6115
	lbsAskGameCode          CmdID = 0x6116
	lbsAskGameVersion       CmdID = 0x6117
	lbsLoginOk              CmdID = 0x6118
	lbsAskBattleResult      CmdID = 0x6120
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
	lbsLobbyEntry         CmdID = 0x6305 // Select team and enter lobby chat scene
	lbsLobbyExit          CmdID = 0x6408 // Exit lobby chat and enter join team select scene
	lbsLobbyMatchingJoin  CmdID = 0x640F
	lbsRoomMax            CmdID = 0x6401
	lbsRoomTitle          CmdID = 0x6402
	lbsRoomStatus         CmdID = 0x6404
	lbsRoomCreate         CmdID = 0x6407
	lbsPutRoomName        CmdID = 0x6609
	lbsEndRoomCreate      CmdID = 0x660C
	lbsRoomEntry          CmdID = 0x6406
	lbsRoomExit           CmdID = 0x6501
	lbsRoomLeaver         CmdID = 0x6502 // A partner left from your room.
	lbsRoomCommer         CmdID = 0x6503 // A player want to enter a room.
	lbsMatchingEntry      CmdID = 0x6504 // Room matching
	lbsRoomRemove         CmdID = 0x6505 // The room manager left from the room.
	lbsWaitJoin           CmdID = 0x6506
	lbsRoomUserReject     CmdID = 0x6507
	lbsPostChatMessage    CmdID = 0x6701
	lbsChatMessage        CmdID = 0x6702
	lbsUserSite           CmdID = 0x6703
	lbsLobbyRemove        CmdID = 0x64C0
	lbsLobbyMatchingEntry CmdID = 0x640E
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

	// gdxsv extended commands
	lbsExtSyncSharedData CmdID = 0x9900
	lbsPlatformInfo      CmdID = 0x9950
)

func RequestLineCheck(p *LbsPeer) {
	p.SendMessage(NewServerQuestion(lbsLineCheck))
}

var _ = register(lbsLineCheck, func(p *LbsPeer, m *LbsMessage) {
	// the client is alive
})

var _ = register(lbsLogout, func(p *LbsPeer, m *LbsMessage) {
	// the client is logging out
	if p.Room != nil {
		p.Room.Exit(p.UserID)
		p.app.BroadcastRoomState(p.Room)
		p.Room = nil
	}
	if p.Lobby != nil {
		p.Lobby.Exit(p.UserID)
		p.app.BroadcastLobbyUserCount(p.Lobby)
		p.Lobby = nil
	}
})

func SendServerShutDown(p *LbsPeer) {
	n := NewServerNotice(lbsShutDown)
	w := n.Writer()
	w.WriteString("<LF=6><BODY><CENTER>サーバがシャットダウンしました<END>")
	p.SendMessage(n)
	p.logger.Info("sending shutdown")
}

func StartLoginFlow(p *LbsPeer) {
	p.SendMessage(NewServerQuestion(lbsAskConnectionID))
}

var _ = register(lbsAskConnectionID, func(p *LbsPeer, m *LbsMessage) {
	p.lastSessionID = m.Reader().ReadString()
	p.SessionID = genSessionID()
	p.logger = p.logger.With(
		zap.String("last_session_id", p.lastSessionID),
		zap.String("session_id", p.SessionID),
	)
	p.logger.Info("update session id")
	p.SendMessage(NewServerQuestion(lbsConnectionID).Writer().
		WriteString(p.SessionID).Msg())
})

var _ = register(lbsConnectionID, func(p *LbsPeer, m *LbsMessage) {
	p.SendMessage(NewServerNotice(lbsWarningMessage).Writer().
		Write8(0).Msg())
})

var _ = register(lbsRegulationHeader, func(p *LbsPeer, m *LbsMessage) {
	p.SendMessage(NewServerAnswer(m).Writer().
		WriteString("1000").
		WriteString("1000").Msg())
	p.SendMessage(NewServerNotice(lbsRegulationText).Writer().
		WriteString("tag").
		WriteString("text").Msg())
	p.SendMessage(NewServerNotice(lbsRegulationFooter))
	p.SendMessage(NewServerQuestion(lbsLoginType))
})

func sendUserList(p *LbsPeer) {
	account, err := getDB().GetAccountBySessionID(p.SessionID)
	if err != nil {
		p.logger.Warn("failed to get account", zap.Error(err))
		p.SendMessage(NewServerNotice(lbsShutDown).Writer().
			WriteString("<LF=5><BODY><CENTER>FAILED TO GET ACCOUNT INFO<END>").Msg())
		return
	}

	users, err := getDB().GetUserList(account.LoginKey)
	if err != nil {
		p.logger.Error("failed to get user list", zap.Error(err))
		p.SendMessage(NewServerNotice(lbsShutDown).Writer().
			WriteString("<LF=5><BODY><CENTER>FAILED TO GET USER LIST<END>").Msg())
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
}

var _ = register(lbsLoginType, func(p *LbsPeer, m *LbsMessage) {
	loginType := m.Reader().Read8()

	// LoginType
	// 0 : 「ネットワーク接続」
	// 1 : 「新規登録」
	// 2 : 「登録情報変更」
	// 3 : The user come back from battle server

	if p.app.IsTempBan(p) {
		p.SendMessage(NewServerNotice(lbsShutDown).Writer().
			WriteString("<LF=5><BODY><CENTER>TEMPORARY BANNED<END>").Msg())
		return
	}

	if p.app.IsBannedEndpoint(p) {
		p.SendMessage(NewServerNotice(lbsShutDown).Writer().
			WriteString("<LF=5><BODY><CENTER>YOU ARE BANNED<END>").Msg())
		return
	}

	switch loginType {
	case 0:
		if p.PlatformInfo["flycast"] != "" {
			if semver.Compare(p.PlatformInfo["flycast"], requiredFlycastVersion) < 0 {
				p.SendMessage(NewServerNotice(lbsShutDown).Writer().
					WriteString("<LF=5><BODY><CENTER>PLEASE UPDATE Flycast<END>").Msg())
				return
			}
		}
		if p.LoginKey != "" {
			// Check login key is banned
			if p.app.IsBannedAccount(p.LoginKey) {
				p.SendMessage(NewServerNotice(lbsShutDown).Writer().
					WriteString("<LF=5><BODY><CENTER>YOU ARE BANNED<END>").Msg())
				return
			}

			// Get account by pre-sent loginkey
			account, err := getDB().GetAccountByLoginKey(p.LoginKey)
			if err != nil {
				p.SendMessage(NewServerNotice(lbsShutDown).Writer().
					WriteString("<LF=5><BODY><CENTER>FAILED TO GET ACCOUNT INFO<END>").Msg())
				return
			}

			// Update session_id that was generated when the first request.
			err = getDB().LoginAccount(account, p.SessionID, p.IP(), p.PlatformInfo["machine_id"])
			if err != nil {
				logger.Error("failed to login account", zap.Error(err))
				p.SendMessage(NewServerNotice(lbsShutDown).Writer().
					WriteString("<LF=5><BODY><CENTER>FAILED TO LOGIN<END>").Msg())
				return
			}
			sendUserList(p)
			return
		}

		p.logger.Info("unsupported login type", zap.Any("login_type", loginType))
		p.SendMessage(NewServerNotice(lbsShutDown).Writer().
			WriteString("<LF=5><BODY><CENTER>UNSUPPORTED LOGIN TYPE<END>").Msg())
	case 2:
		if p.PlatformInfo["flycast"] != "" {
			if semver.Compare(p.PlatformInfo["flycast"], requiredFlycastVersion) < 0 {
				p.SendMessage(NewServerNotice(lbsShutDown).Writer().
					WriteString("<LF=5><BODY><CENTER>PLEASE UPDATE Flycast<END>").Msg())
				return
			}
		}

		// Go to account registration flow.
		p.SendMessage(NewServerQuestion(lbsUserInfo1))
	case 3:
		// The user must have valid connection_id.
		if p.lastSessionID == "" {
			p.SendMessage(NewServerNotice(lbsShutDown).Writer().
				WriteString("<LF=5><BODY><CENTER>INVALID CONNECTION ID<END>").Msg())
			return
		}

		account, err := getDB().GetAccountBySessionID(p.lastSessionID)
		if err != nil {
			p.SendMessage(NewServerNotice(lbsShutDown).Writer().
				WriteString("<LF=5><BODY><CENTER>FAILED TO GET ACCOUNT<END>").Msg())
			return
		}

		// Update session_id that was generated when the first request.
		err = getDB().LoginAccount(account, p.SessionID, p.IP(), p.PlatformInfo["machine_id"])
		if err != nil {
			p.SendMessage(NewServerNotice(lbsShutDown).Writer().
				WriteString("<LF=5><BODY><CENTER>FAILED TO LOGIN<END>").Msg())
			return
		}

		sendUserList(p)
	default:
		p.logger.Info("unsupported login type", zap.Any("login_type", loginType))
		p.SendMessage(NewServerNotice(lbsShutDown).Writer().
			WriteString("<LF=5><BODY><CENTER>UNSUPPORTED LOGIN TYPE<END>").Msg())
	}
})

var _ = register(lbsEncodeStart, func(p *LbsPeer, m *LbsMessage) {
	// Client sends this packet before sending user personal info.
	// There is no special information.
})

var _ = register(lbsUserInfo1, func(p *LbsPeer, m *LbsMessage) {
	// Calculate hash value of telephone number that has been treated simple encryption,
	// and use it as login_key.
	// If user send same telephone number same login key must be generated.
	hasher := fnv.New32()
	hasher.Write(m.Reader().ReadBytes())
	loginKey := hex.EncodeToString(hasher.Sum(nil))

	if p.app.IsBannedAccount(loginKey) {
		p.SendMessage(NewServerNotice(lbsShutDown).Writer().
			WriteString("<LF=5><BODY><CENTER>YOU ARE BANNED<END>").Msg())
		return
	}

	// If the user already have an account, get it.
	account, err := getDB().GetAccountByLoginKey(loginKey)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			account, err = getDB().RegisterAccountWithLoginKey(p.IP(), loginKey)
			if err != nil {
				p.logger.Error("failed to create account", zap.Error(err))
				p.SendMessage(NewServerNotice(lbsShutDown).Writer().
					WriteString("<LF=5><BODY><CENTER>FAILED TO GET ACCOUNT INFO<END>").Msg())
				return
			}
		default:
			p.logger.Error("failed to get account", zap.Error(err))
			p.SendMessage(NewServerNotice(lbsShutDown).Writer().
				WriteString("<LF=5><BODY><CENTER>FAILED TO GET ACCOUNT INFO<END>").Msg())
			return
		}
	}

	// Now the user has valid account.
	// Update session_id that was generated when the first request.
	err = getDB().LoginAccount(account, p.SessionID, p.IP(), p.PlatformInfo["machine_id"])
	if err != nil {
		logger.Error("failed to login account", zap.Error(err))
		p.SendMessage(NewServerNotice(lbsShutDown).Writer().
			WriteString("<LF=5><BODY><CENTER>FAILED TO LOGIN<END>").Msg())
		return
	}

	p.SendMessage(NewServerQuestion(lbsUserInfo9))
})

var _ = register(lbsUserInfo9, func(p *LbsPeer, m *LbsMessage) {
	sendUserList(p)
})

var _ = register(lbsUserRegist, func(p *LbsPeer, m *LbsMessage) {
	r := m.Reader()
	userID := r.ReadString()
	handleName := r.ReadShiftJISString()
	logger.Info("create new user", zap.String("user_id", userID), zap.String("handle_name", handleName))

	account, err := getDB().GetAccountBySessionID(p.SessionID)
	if err != nil {
		p.logger.Error("failed to get account", zap.Error(err))
		p.conn.Close()
		return
	}

	if userID == "******" {
		// The peer wants to create new user.
		p.logger.Info("register new user")
		u, err := getDB().RegisterUser(account.LoginKey)
		if err != nil {
			p.logger.Error("failed to register user :", zap.Error(err))
			p.conn.Close()
			return
		}
		userID = u.UserID
	}

	u, err := getDB().GetUser(userID)
	if err != nil {
		logger.Error("failed to get user :", zap.Error(err), zap.String("user_id", userID))
		p.conn.Close()
		return
	}

	err = getDB().LoginUser(u)
	if err != nil {
		logger.Error("failed to login user", zap.Error(err), zap.String("user_id", userID))
		p.conn.Close()
		return
	}

	u.Name = handleName
	u.SessionID = p.SessionID
	err = getDB().UpdateUser(u)
	if err != nil {
		p.logger.Error("failed to update user", zap.Error(err), zap.String("user_id", userID))
		p.conn.Close()
		return
	}

	p.DBUser = *u
	p.app.userPeers[p.UserID] = p
	p.logger = p.logger.With(zap.String("user_id", p.UserID), zap.String("handle_name", p.Name))
	p.SendMessage(NewServerAnswer(m).Writer().WriteString(userID).Msg())
})

var _ = register(lbsUserDecide, func(p *LbsPeer, m *LbsMessage) {
	userID := m.Reader().ReadString()

	u, err := getDB().GetUser(userID)
	if err != nil {
		p.logger.Error("failed to get user", zap.String("user_id", userID), zap.Error(err))
		p.conn.Close()
		return
	}

	err = getDB().LoginUser(u)
	if err != nil {
		p.logger.Error("failed to login user", zap.String("user_id", userID), zap.Error(err))
		p.conn.Close()
		return
	}

	u.SessionID = p.SessionID
	err = getDB().UpdateUser(u)
	if err != nil {
		p.logger.Error("failed to update user", zap.String("user_id", userID), zap.Error(err))
		p.conn.Close()
		return
	}

	p.DBUser = *u
	p.app.userPeers[p.UserID] = p
	p.logger = p.logger.With(
		zap.String("user_id", p.UserID),
		zap.String("handle_name", p.Name),
	)
	p.SendMessage(NewServerAnswer(m).Writer().WriteString(p.UserID).Msg())
	p.SendMessage(NewServerQuestion(lbsAskGameCode))
})

var _ = register(lbsAskGameCode, func(p *LbsPeer, m *LbsMessage) {
	code := 0
	if m.BodySize == 1 {
		code = int(m.Reader().Read8())
	} else {
		code = int(m.Reader().Read16())
	}

	switch code {
	case 0x02:
		p.GameDisk = GameDiskPS2
	case 0x0300:
		p.GameDisk = GameDiskDC1
	case 0x0301:
		p.GameDisk = GameDiskDC2
	default:
		p.logger.Warn("unknown client platform", zap.Any("code", code))
		p.SendMessage(NewServerNotice(lbsShutDown).Writer().
			WriteString("<LF=5><BODY><CENTER>UNKNOWN CLIENT PLATFORM<END>").Msg())
		return
	}

	p.SendMessage(NewServerQuestion(lbsAskBattleResult))
})

var _ = register(lbsAskBattleResult, func(p *LbsPeer, m *LbsMessage) {
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
	p.app.RegisterBattleResult(p, result)
	p.SendMessage(NewServerNotice(lbsLoginOk))
	p.logger.Info("login ok")
})

var _ = register(lbsPostGameParameter, func(p *LbsPeer, m *LbsMessage) {
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
		logger.Error("failed to read pilot name", zap.Error(err))
	}
	p.PilotName = string(bin)
	p.logger = p.logger.With(zap.String("pilot_name", p.PilotName))
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(lbsAskKDDICharges, func(p *LbsPeer, m *LbsMessage) {
	// 課金予測情報 (円)
	p.SendMessage(NewServerAnswer(m).Writer().Write32(0).Msg())
})

var _ = register(lbsAskNewsTag, func(p *LbsPeer, m *LbsMessage) {
	a := NewServerAnswer(m)
	w := a.Writer()

	newsTag, err := getDB().GetString("news_tag")
	if err != nil {
		logger.Error("failed to get news_tag", zap.Error(err))
	}

	if newsTag == "" {
		w.Write8(0)       // news count
		w.WriteString("") // news_tag
	} else {
		w.Write8(1)
		w.WriteString(newsTag)
	}
	p.SendMessage(a)
})

var _ = register(lbsNewsText, func(p *LbsPeer, m *LbsMessage) {
	// m.ReadString() // unknown
	newsText, err := getDB().GetString("news_text")
	if err != nil {
		logger.Error("failed to get news_text", zap.Error(err))
	}
	a := NewServerAnswer(m)
	w := a.Writer()
	w.WriteString(newsText)
	p.SendMessage(a)
})

var _ = register(lbsAskPatchData, func(p *LbsPeer, m *LbsMessage) {
	r := m.Reader()
	platform := r.Read8()
	crule := r.Read8()
	data := r.ReadString()
	_, _, _ = platform, crule, data

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
			grade = 14 // 1~5 [大将]
		} else if rank <= 20 {
			grade = 13 // 6~20 [中将]
		} else if rank <= 50 {
			grade = 12 // 21~50 [少将]
		} else {
			grade = 11 // 50~ [大佐]
		}
	}

	return uint8(grade)
}

var _ = register(lbsRankRanking, func(p *LbsPeer, m *LbsMessage) {
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

var _ = register(lbsWinLose, func(p *LbsPeer, m *LbsMessage) {
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
		p.logger.Warn("unknown top rank", zap.Any("now_top_rank", nowTopRank))
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

var _ = register(lbsDeviceData, func(p *LbsPeer, m *LbsMessage) {
	r := m.Reader()
	data1 := r.Read16()
	data2 := r.Read16()
	data3 := r.Read16()
	data4 := r.Read16()
	data5 := r.Read16()
	data6 := r.Read16()
	data7 := r.Read16()
	data8 := r.Read16()
	p.logger.Sugar().Info("DeviceData", data1, data2, data3, data4, data5, data6, data7, data8)
	// PS2: 0 0 0 999 1 0 0 0
	// DC1: 0 0 0 0 1 0 0 0
	// DC2: 0 0 0 0 1 0 0 0

	p.SendMessage(NewServerAnswer(m))
})

var _ = register(lbsServerMoney, func(p *LbsPeer, m *LbsMessage) {
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(0).Write16(0).Write16(0).Write16(0).Msg())
})

var _ = register(lbsStartLobby, func(p *LbsPeer, m *LbsMessage) {
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(lbsInvitationTag, func(p *LbsPeer, m *LbsMessage) {
	p.SendMessage(NewServerAnswer(m).Writer().
		WriteString("tabbuf").
		WriteString("invitation").
		Write8(0).Msg())
})

var _ = register(lbsPlazaMax, func(p *LbsPeer, m *LbsMessage) {
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(maxLobbyCount).Msg())
})

var _ = register(lbsPlazaJoin, func(p *LbsPeer, m *LbsMessage) {
	lobbyID := m.Reader().Read16()
	// PS2: LobbyID, UserCount
	// DC : LobbyID, DC1UserCount, DC2UserCount
	if p.IsPS2() {
		lobby := p.app.GetLobby(p.Platform, p.GameDisk, lobbyID)
		if lobby == nil {
			p.SendMessage(NewServerAnswer(m).SetErr())
			return
		}
		p.SendMessage(NewServerAnswer(m).Writer().
			Write16(lobbyID).
			Write16(uint16(len(lobby.Users))).Msg())
	} else if p.IsDC() {
		lobby1 := p.app.GetLobby(p.Platform, GameDiskDC1, lobbyID)
		lobby2 := p.app.GetLobby(p.Platform, GameDiskDC2, lobbyID)
		if lobby1 == nil || lobby2 == nil {
			p.SendMessage(NewServerAnswer(m).SetErr())
			return
		}
		p.SendMessage(NewServerAnswer(m).Writer().
			Write16(lobbyID).
			Write16(uint16(len(lobby1.Users))).
			Write16(uint16(len(lobby2.Users))).Msg())
	} else {
		p.SendMessage(NewServerAnswer(m).SetErr())
	}
})

var _ = register(lbsPlazaStatus, func(p *LbsPeer, m *LbsMessage) {
	lobbyID := m.Reader().Read16()
	lobby := p.app.GetLobby(p.Platform, p.GameDisk, lobbyID)
	if lobby == nil {
		p.SendMessage(NewServerAnswer(m).Writer().
			Write16(lobbyID).
			Write8(uint8(0)).Msg())
		return
	}

	status := 3 // available
	if lobby.LobbySetting.PingLimit && lobby.LobbySetting.McsRegion != "" {
		rtt, err := strconv.Atoi(p.PlatformInfo[lobby.LobbySetting.McsRegion])
		if err != nil {
			rtt = 999
		}
		if rtt <= 0 || PingLimitTh <= rtt {
			status = 1 // not available
		}
	}

	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(lobbyID).
		Write8(uint8(status)).Msg())
})

var _ = register(lbsPlazaExplain, func(p *LbsPeer, m *LbsMessage) {
	lobbyID := m.Reader().Read16()
	lobby := p.app.GetLobby(p.Platform, p.GameDisk, lobbyID)
	if lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	rtt := p.PlatformInfo[lobby.LobbySetting.McsRegion]
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(lobbyID).
		WriteString(lobby.buildDescription(rtt)).
		Msg())
})

var _ = register(lbsPlazaEntry, func(p *LbsPeer, m *LbsMessage) {
	lobbyID := m.Reader().Read16()
	lobby := p.app.GetLobby(p.Platform, p.GameDisk, lobbyID)
	if lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	p.Lobby = lobby
	p.Team = TeamNone

	lobby.Enter(p)
	p.SendMessage(NewServerAnswer(m))
	p.app.BroadcastLobbyUserCount(lobby)
})

var _ = register(lbsPlazaExit, func(p *LbsPeer, m *LbsMessage) {
	if p.Lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	lobby := p.Lobby

	p.Lobby.Exit(p.UserID)
	p.Lobby = nil
	p.Team = TeamNone

	p.SendMessage(NewServerAnswer(m))
	p.app.BroadcastLobbyUserCount(lobby)
})

var _ = register(lbsLobbyEntry, func(p *LbsPeer, m *LbsMessage) {
	if p.Lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	team := m.Reader().Read16()
	p.Team = team
	p.SendMessage(NewServerAnswer(m))
	p.app.BroadcastLobbyUserCount(p.Lobby)
	p.Lobby.SwitchTeam(p)
})

var _ = register(lbsLobbyExit, func(p *LbsPeer, m *LbsMessage) {
	if p.Lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	// LobbyExit means go back to team select scene.
	// So don't remove Lobby ref here.

	p.Team = TeamNone

	p.SendMessage(NewServerAnswer(m))
	p.app.BroadcastLobbyUserCount(p.Lobby)
	p.Lobby.SwitchTeam(p)
})

var _ = register(lbsLobbyJoin, func(p *LbsPeer, m *LbsMessage) {
	if p.Lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	team := m.Reader().Read16()
	switch p.Lobby.GameDisk {
	case GameDiskPS2:
		renpo, zeon := p.Lobby.GetUserCountByTeam()
		if p.InLobbyChat() {
			p.SendMessage(NewServerAnswer(m).Writer().
				Write16(team).Write16(renpo + zeon).Msg())
		} else {
			if team == 1 {
				p.SendMessage(NewServerAnswer(m).Writer().
					Write16(team).Write16(renpo).Msg())
			} else {
				p.SendMessage(NewServerAnswer(m).Writer().
					Write16(team).Write16(zeon).Msg())
			}
		}
	case GameDiskDC1, GameDiskDC2:
		lobby1 := p.app.GetLobby(p.Lobby.Platform, GameDiskDC1, p.Lobby.ID)
		lobby2 := p.app.GetLobby(p.Lobby.Platform, GameDiskDC2, p.Lobby.ID)
		if lobby1 == nil || lobby2 == nil {
			p.SendMessage(NewServerAnswer(m).SetErr())
			return
		}

		renpo1, zeon1 := lobby1.GetUserCountByTeam()
		renpo2, zeon2 := lobby2.GetUserCountByTeam()
		if p.InLobbyChat() {
			p.SendMessage(NewServerAnswer(m).Writer().
				Write16(team).
				Write16(renpo1 + zeon1).
				Write16(renpo2 + zeon2).Msg())
		} else {
			if team == 1 {
				p.SendMessage(NewServerAnswer(m).Writer().
					Write16(team).
					Write16(renpo1).
					Write16(renpo2).Msg())
			} else {
				p.SendMessage(NewServerAnswer(m).Writer().
					Write16(team).
					Write16(zeon1).
					Write16(zeon2).Msg())
			}
		}
	}
})

var _ = register(lbsLobbyMatchingJoin, func(p *LbsPeer, m *LbsMessage) {
	if p.Lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	team := m.Reader().Read16()
	renpo, zeon := p.Lobby.GetLobbyMatchEntryUserCount()
	if team == 1 {
		p.SendMessage(NewServerAnswer(m).Writer().
			Write16(team).Write16(renpo).Msg())
	} else {
		p.SendMessage(NewServerAnswer(m).Writer().
			Write16(team).Write16(zeon).Msg())
	}
})

var _ = register(lbsLobbyMatchingEntry, func(p *LbsPeer, m *LbsMessage) {
	if p.Lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	enable := m.Reader().Read8()
	if enable == 1 {
		p.Lobby.Entry(p)
	} else {
		p.Lobby.EntryCancel(p)
	}
	p.SendMessage(NewServerAnswer(m))
	p.app.BroadcastLobbyMatchEntryUserCount(p.Lobby)
})

var _ = register(lbsRoomStatus, func(p *LbsPeer, m *LbsMessage) {
	if p.Lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	roomID := m.Reader().Read16()
	room := p.Lobby.FindRoom(p.Team, roomID)
	if room == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(roomID).
		Write8(room.Status).Msg())
})

var _ = register(lbsRoomMax, func(p *LbsPeer, m *LbsMessage) {
	if p.Lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}
	p.SendMessage(NewServerAnswer(m).Writer().Write16(maxRoomCount).Msg())
})

var _ = register(lbsRoomTitle, func(p *LbsPeer, m *LbsMessage) {
	if p.Lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	roomID := m.Reader().Read16()
	room := p.Lobby.FindRoom(p.Team, roomID)
	if room == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(roomID).
		WriteString(room.Name).Msg())
})

var _ = register(lbsRoomCreate, func(p *LbsPeer, m *LbsMessage) {
	if p.Lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	roomID := m.Reader().Read16()
	room := p.Lobby.FindRoom(p.Team, roomID)
	if room == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	if room.Status != RoomStateEmpty {
		p.SendMessage(NewServerAnswer(m).SetErr().Writer().
			WriteString("<LF=6><BODY>Failed to create room<END>").Msg())
		return
	}

	room.Status = RoomStatePrepare
	room.Owner = p.UserID
	p.Room = room
	p.SendMessage(NewServerAnswer(m))
	p.app.BroadcastRoomState(room)
})

var _ = register(lbsPutRoomName, func(p *LbsPeer, m *LbsMessage) {
	if p.Room == nil || p.Room.Owner != p.UserID || p.Room.Status != RoomStatePrepare {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	roomName := m.Reader().ReadShiftJISString()
	p.Room.Name = roomName
	p.SendMessage(NewServerAnswer(m))
	p.app.BroadcastRoomState(p.Room)
})

var _ = register(lbsEndRoomCreate, func(p *LbsPeer, m *LbsMessage) {
	if p.Room == nil || p.Room.Owner != p.UserID || p.Room.Status != RoomStatePrepare {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	p.Room.Enter(&p.DBUser)

	p.SendMessage(NewServerAnswer(m))
	p.app.BroadcastRoomState(p.Room)
})

var _ = register(lbsSendMail, func(p *LbsPeer, m *LbsMessage) {
	r := m.Reader()
	userID := r.ReadString()
	comment1 := r.ReadShiftJISString()
	comment2 := r.ReadShiftJISString()

	logger.Info("send mail",
		zap.String("to_user_id", userID),
		zap.String("comment1", comment1),
		zap.String("comment2", comment2))

	u, ok := p.app.userPeers[userID]
	if !ok {
		p.SendMessage(NewServerAnswer(m).SetErr().Writer().
			WriteString("<LF=6><BODY><CENTER>THE USER IS NOT IN LOBBY<END>").Msg())
		return
	}

	u.SendMessage(NewServerNotice(lbsRecvMail).Writer().
		WriteString(p.UserID).
		WriteString(p.Name).
		WriteString(comment1).Msg())
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(lbsUserSite, func(p *LbsPeer, m *LbsMessage) {
	// TODO: Implement
	userID := m.Reader().ReadString()
	_ = userID
	logger.Warn("not implemented lbsUserSite")
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(0).
		Write16(1).
		Write16(2).
		Write8(3).
		Write8(4).
		Write8(5).
		WriteString("<LF=6><BODY><CENTER>UNDER CONSTRUCTION<END>").Msg())
})

var _ = register(lbsWaitJoin, func(p *LbsPeer, m *LbsMessage) {
	waiting := uint16(0)
	if p.Room != nil && p.Room.Status == RoomStateRecruiting {
		waiting = 1
	}
	if p.Room != nil && p.Room.Status == RoomStateFull {
		waiting = 2
	}
	p.SendMessage(NewServerAnswer(m).Writer().Write16(waiting).Msg())
})

var _ = register(lbsRoomEntry, func(p *LbsPeer, m *LbsMessage) {
	if p.Lobby == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	r := m.Reader()
	roomID := r.Read16()
	_ = r.Read16() // unknown

	room := p.Lobby.FindRoom(p.Team, roomID)
	if room == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	if room.Status != RoomStateRecruiting {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	room.Enter(&p.DBUser)
	p.Room = room
	for _, u := range room.Users {
		if q := p.app.FindPeer(u.UserID); q != nil {
			q.SendMessage(NewServerNotice(lbsRoomCommer).Writer().
				WriteString(p.UserID).
				WriteString(p.Name).Msg())
		}
	}
	p.SendMessage(NewServerAnswer(m))
	p.app.BroadcastRoomState(room)
})

var _ = register(lbsRoomUserReject, func(p *LbsPeer, m *LbsMessage) {
	userID := m.Reader().ReadString()
	p.SendMessage(NewServerAnswer(m))

	if p.Room == nil {
		return
	}

	if p.Room.Owner != p.UserID {
		return
	}

	q := p.app.FindPeer(userID)
	if q == nil {
		return
	}

	if q.Room != p.Room {
		return
	}

	q.SendMessage(NewServerNotice(lbsRoomRemove).Writer().
		WriteString("<LF=6><BODY><CENTER>拒否されました。<END>").Msg())
})

var _ = register(lbsRoomExit, func(p *LbsPeer, m *LbsMessage) {
	defer p.SendMessage(NewServerAnswer(m))

	if p.Room == nil {
		return
	}

	r := p.Room
	p.Room = nil

	if r.Owner == p.UserID {
		for _, u := range r.Users {
			if r.Owner != u.UserID {
				if q := p.app.FindPeer(u.UserID); q != nil {
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
			if q := p.app.FindPeer(u.UserID); q != nil {
				q.SendMessage(NewServerNotice(lbsWaitJoin).Writer().Write16(1).Msg())
				q.SendMessage(NewServerNotice(lbsRoomLeaver).Writer().
					WriteString(p.UserID).
					WriteString(p.Name).Msg())
			}
		}
	}

	p.app.BroadcastRoomState(r)
})

var _ = register(lbsMatchingEntry, func(p *LbsPeer, m *LbsMessage) {
	if p.Room == nil {
		// WORKAROUND FIX
		// client freezes when two users trying to leave a room at the same time.
		p.SendMessage(NewServerNotice(lbsWaitJoin).Writer().Write16(0).Msg())
		p.SendMessage(NewServerAnswer(m))
		return
	}

	r := p.Room
	enable := m.Reader().Read8()

	if r.Owner == p.UserID {
		r.Ready(p, enable)
	} else if enable == 0 {
		for _, u := range r.Users {
			if q := p.app.FindPeer(u.UserID); p != nil {
				q.SendMessage(NewServerNotice(lbsWaitJoin).Writer().Write16(1).Msg())
				q.SendMessage(NewServerNotice(lbsRoomLeaver).Writer().
					WriteString(p.UserID).
					WriteString(p.Name).Msg())
			}
		}
		r.Exit(p.UserID)
		p.Room = nil
		p.SendMessage(NewServerNotice(lbsWaitJoin).Writer().Write16(0).Msg())
	}
	p.SendMessage(NewServerAnswer(m))
})

var _ = register(lbsPostChatMessage, func(p *LbsPeer, m *LbsMessage) {
	text := m.Reader().ReadShiftJISString()
	msg := NewServerNotice(lbsChatMessage).Writer().
		WriteString(p.UserID).
		WriteString(p.Name).
		WriteString(text).
		Write8(0). // chat_type
		Write8(0). // id color
		Write8(0). // handle color
		Write8(0).Msg() // msg color

	// broadcast chat message to users in the same place.
	if p.Room != nil {
		for _, u := range p.Room.Users {
			if q := p.app.FindPeer(u.UserID); q != nil {
				q.SendMessage(msg)
			}
		}
	} else if p.Lobby != nil {
		for _, u := range p.Lobby.Users {
			if q := p.app.FindPeer(u.UserID); q != nil {
				if q.InLobbyChat() {
					q.SendMessage(msg)
				}
			}
		}
	}

	// Additional actions.
	if text == "／ｆ" || text == "／Ｆ" {
		buildHintMsg := func(hint string) *LbsMessage {
			return NewServerNotice(lbsChatMessage).Writer().
				WriteString("").
				WriteString("").
				WriteString(hint).
				Write8(0). // chat_type
				Write8(0). // id color
				Write8(0). // handle color
				Write8(0).Msg() // msg color
		}

		//intercept message if it is a command
		userHasJoinedForce := false
		for _, userID := range p.Lobby.EntryUsers {
			if p.UserID == userID {
				userHasJoinedForce = true
				break
			}
		}
		twoOrMorePlayers := len(p.Lobby.EntryUsers) >= 2

		if p.Lobby.LobbySetting.EnableForceStart && userHasJoinedForce && twoOrMorePlayers {
			// Print induced action to all users (for clarity + educational purpose)
			p.Lobby.StartForceStartCountDown()
			p.Lobby.NotifyLobbyEvent("", fmt.Sprintf("%v starts battle countdown!", p.Name))
		} else {
			// Print hints to sender
			if !p.Lobby.LobbySetting.EnableForceStart {
				p.SendMessage(buildHintMsg("/f is disabled in this lobby"))
			} else if !userHasJoinedForce {
				p.SendMessage(buildHintMsg("Join a force first! (自動選抜 -> 待機)"))
			} else if !twoOrMorePlayers {
				p.SendMessage(buildHintMsg("Battle requires at least 2 players!"))
			}

		}
	}
})

var _ = register(lbsTopRankingTag, func(p *LbsPeer, m *LbsMessage) {
	topRankSuu := uint8(1)
	topRankTag := "勝利数ランキング"
	p.SendMessage(NewServerAnswer(m).Writer().
		Write8(topRankSuu).
		WriteString(topRankTag).Msg())
})

var _ = register(lbsTopRankingSuu, func(p *LbsPeer, m *LbsMessage) {
	// How many userPeers there is in the ranking
	// page: ranking kind?
	page := m.Reader().Read8()
	p.logger.Info("lbsTopRankingSuu", zap.Any("page", page))

	n := 0
	if ranking, err := getDB().GetWinCountRanking(0); err == nil {
		n = len(ranking)
	}
	p.SendMessage(NewServerAnswer(m).Writer().Write16(uint16(n)).Msg())
})

var _ = register(lbsTopRanking, func(p *LbsPeer, m *LbsMessage) {
	r := m.Reader()
	num1 := r.Read8()
	num2 := r.Read16()
	num3 := r.Read16()
	p.logger.Sugar().Info("TopRanking", num1, num2, num3)

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
	// 1: Red
	// 2: Green
	// 3: Yellow
	// 4: Blue
	// 5: Purple
	// 6: SkyBlue
	// 7: White

	rec := ranking[index]
	topRankerNum := uint16(num2)
	topRankStr := "" +
		fmt.Sprintf(
			"<COLOR=7><SIZE=4><BODY>%3d位 <COLOR=3> %s <COLOR=6>%v<BR>",
			rec.Rank, width.Widen.String(rec.UserID), rec.Name) +
		fmt.Sprintf("<SIZE=3><COLOR=7>%5d<COLOR=3>戦<COLOR=7> %5d<COLOR=3>勝<COLOR=7> %5d<COLOR=3>敗<COLOR=7> %5d<COLOR=3>無効<COLOR=7><END>",
			rec.WinCount+rec.LoseCount, rec.WinCount, rec.LoseCount, rec.BattleCount-rec.WinCount-rec.LoseCount)
	p.SendMessage(NewServerAnswer(m).Writer().
		Write16(topRankerNum).
		WriteString(topRankStr).Msg())
})

var _ = register(lbsGoToTop, func(p *LbsPeer, m *LbsMessage) {
	room := p.Room
	lobby := p.Lobby

	if room != nil {
		room.Exit(p.UserID)
	}

	if p.Lobby != nil {
		lobby.Exit(p.UserID)
	}

	p.Room = nil
	p.Lobby = nil
	p.Battle = nil
	p.Team = TeamNone

	p.SendMessage(NewServerAnswer(m))

	p.app.BroadcastLobbyUserCount(lobby)
	p.app.BroadcastLobbyMatchEntryUserCount(lobby)
	p.app.BroadcastRoomState(room)
})

func NotifyReadyBattle(p *LbsPeer) {
	p.SendMessage(NewServerNotice(lbsReadyBattle))
}

var _ = register(lbsAskMatchingJoin, func(p *LbsPeer, m *LbsMessage) {
	// how many players in the game
	n := p.Battle.NumOfEntryUsers()
	p.SendMessage(NewServerAnswer(m).Writer().Write8(uint8(n)).Msg())
})

var _ = register(lbsAskPlayerSide, func(p *LbsPeer, m *LbsMessage) {
	// player position
	p.SendMessage(NewServerAnswer(m).Writer().Write8(p.Battle.GetPosition(p.UserID)).Msg())
})

func r16(a int) uint16 {
	if math.MaxUint16 < a {
		return math.MaxUint16
	}
	return uint16(a)
}

var _ = register(lbsAskPlayerInfo, func(p *LbsPeer, m *LbsMessage) {
	if p.Battle == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	pos := m.Reader().Read8()
	u := p.Battle.GetUserByPos(pos)
	param := p.Battle.GetGameParamByPos(pos)
	team := p.Battle.GetUserTeam(u.UserID)
	grade := decideGrade(u.WinCount, p.Battle.GetUserRankByPos(pos))
	msg := NewServerAnswer(m).Writer().
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
		Write16(team).
		Write16(0). // Unknown
		Msg()
	p.SendMessage(msg)
})

var _ = register(lbsAskRuleData, func(p *LbsPeer, m *LbsMessage) {
	if p.Battle == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	// Binary rule data
	// 001e2980: NetRecvHeyaBinDef default values
	// 001e2830: NetHeyaDataSet    overwrite ?
	a := NewServerAnswer(m)
	w := a.Writer()
	bin := SerializeRule(p.Battle.Rule)
	w.Write16(uint16(len(bin)))
	w.Write(bin)
	p.SendMessage(a)
})

var _ = register(lbsAskBattleCode, func(p *LbsPeer, m *LbsMessage) {
	if p.Battle == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	p.SendMessage(NewServerAnswer(m).Writer().WriteString(p.Battle.BattleCode).Msg())
})

var _ = register(lbsAskMcsAddress, func(p *LbsPeer, m *LbsMessage) {
	if p.Battle == nil {
		p.SendMessage(NewServerAnswer(m).SetErr())
		return
	}

	ip := p.Battle.ServerIP
	port := p.Battle.ServerPort

	if ip == nil || ip.To4() == nil || port == 0 {
		logger.Error("invalid mcs address", zap.Any("ip", ip), zap.Any("port", port))
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
	w.Write16(2)
	w.Write16(port)

	p.SendMessage(a)
})

var _ = register(lbsAskMcsVersion, func(p *LbsPeer, m *LbsMessage) {
	p.SendMessage(NewServerAnswer(m).Writer().Write8(10).Msg())
})

var _ = register(lbsExtSyncSharedData, func(p *LbsPeer, m *LbsMessage) {
	body := m.Reader().ReadBytes() // gzipped json
	gr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		p.logger.Error("gzip.NewReader", zap.Error(err), zap.Binary("body", body))
		return
	}

	var mcsStatus McsStatus
	jr := json.NewDecoder(gr)
	err = jr.Decode(&mcsStatus)
	if err != nil {
		p.logger.Error("failed to unmarshal mcs status", zap.Error(err), zap.Binary("body", body))
		return
	}

	p.logger.Info("update mcs status", zap.Any("mcs_status", mcsStatus))
	p.app.mcsPeers[mcsStatus.PublicAddr] = p
	p.mcsStatus = &mcsStatus
	sharedData.SyncMcsToLbs(&mcsStatus)
})

var _ = register(lbsPlatformInfo, func(p *LbsPeer, m *LbsMessage) {
	// patched client sends client-platform information
	r := m.Reader()
	platformInfo := r.ReadString()
	for _, line := range strings.Split(strings.TrimSuffix(platformInfo, "\n"), "\n") {
		kv := strings.SplitN(line, "=", 2)
		if len(kv) == 2 {
			p.PlatformInfo[kv[0]] = kv[1]
		}
	}
	logger.Info("PlatformInfo", zap.Any("platform_info", p.PlatformInfo))
	p.logger = p.logger.With(
		zap.String("flycast", p.PlatformInfo["flycast"]),
		zap.String("os", p.PlatformInfo["os"]),
		zap.String("cpu", p.PlatformInfo["cpu"]),
	)
	if p.PlatformInfo["cpu"] == "x86/64" {
		p.Platform = PlatformEmuX8664
	}

	// pre-sent loginkey
	if 0 < r.Remaining() {
		hasher := fnv.New32()
		hasher.Write(r.ReadBytes())
		loginKey := hex.EncodeToString(hasher.Sum(nil))
		if p.LoginKey == "" {
			p.LoginKey = loginKey
		}
	}
})
