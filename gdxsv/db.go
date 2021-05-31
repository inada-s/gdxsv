package main

import (
	"bytes"
	"math/rand"
	"time"
)

func randomString(length int, source string) string {
	var result bytes.Buffer
	for i := 0; i < length; i++ {
		index := rand.Intn(len(source))
		result.WriteByte(source[index])
	}
	return result.String()
}

func genLoginKey() string {
	return randomString(10, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
}

func genUserID() string {
	return randomString(6, "ABCDEFGHIJKLMNOPQRSTUVWXYZ23456789")
}

func genSessionID() string {
	return randomString(8, "123456789")
}

type DBAccount struct {
	LoginKey           string    `db:"login_key" json:"login_key,omitempty"`
	SessionID          string    `db:"session_id" json:"session_id,omitempty"`
	LastUserID         string    `db:"last_user_id" json:"last_user_id,omitempty"`
	Created            time.Time `db:"created" json:"created,omitempty"`
	CreatedIP          string    `db:"created_ip" json:"created_ip,omitempty"`
	LastLogin          time.Time `db:"last_login" json:"last_login,omitempty"`
	LastLoginIP        string    `db:"last_login_ip" json:"last_login_ip,omitempty"`
	LastLoginMachineID string    `db:"last_login_machine_id" json:"last_login_machine_id,omitempty"`
	System             byte      `db:"system" json:"system,omitempty"`
}

type DBUser struct {
	LoginKey  string `db:"login_key" json:"login_key,omitempty"`
	SessionID string `db:"session_id" json:"session_id,omitempty"`

	UserID string `db:"user_id" json:"user_id,omitempty"`
	Name   string `db:"name" json:"name,omitempty"`
	Team   string `db:"team" json:"team,omitempty"`

	BattleCount      int `db:"battle_count" json:"battle_count,omitempty"`
	WinCount         int `db:"win_count" json:"win_count,omitempty"`
	LoseCount        int `db:"lose_count" json:"lose_count,omitempty"`
	KillCount        int `db:"kill_count" json:"kill_count,omitempty"`
	DeathCount       int `db:"death_count" json:"death_count,omitempty"`
	RenpoBattleCount int `db:"renpo_battle_count" json:"renpo_battle_count,omitempty"`
	RenpoWinCount    int `db:"renpo_win_count" json:"renpo_win_count,omitempty"`
	RenpoLoseCount   int `db:"renpo_lose_count" json:"renpo_lose_count,omitempty"`
	RenpoKillCount   int `db:"renpo_kill_count" json:"renpo_kill_count,omitempty"`
	RenpoDeathCount  int `db:"renpo_death_count" json:"renpo_death_count,omitempty"`
	ZeonBattleCount  int `db:"zeon_battle_count" json:"zeon_battle_count,omitempty"`
	ZeonWinCount     int `db:"zeon_win_count" json:"zeon_win_count,omitempty"`
	ZeonLoseCount    int `db:"zeon_lose_count" json:"zeon_lose_count,omitempty"`
	ZeonKillCount    int `db:"zeon_kill_count" json:"zeon_kill_count,omitempty"`
	ZeonDeathCount   int `db:"zeon_death_count" json:"zeon_death_count,omitempty"`

	DailyBattleCount int `db:"daily_battle_count" json:"daily_battle_count,omitempty"`
	DailyWinCount    int `db:"daily_win_count" json:"daily_win_count,omitempty"`
	DailyLoseCount   int `db:"daily_lose_count" json:"daily_lose_count,omitempty"`

	Created time.Time `db:"created" json:"created,omitempty"`
	System  uint32    `db:"system" json:"system,omitempty"`
}

type BattleRecord struct {
	BattleCode string `db:"battle_code" json:"battle_code,omitempty"`
	UserID     string `db:"user_id" json:"user_id,omitempty"`
	UserName   string `db:"user_name" json:"user_name,omitempty"`
	PilotName  string `db:"pilot_name" json:"pilot_name,omitempty"`
	LobbyID    int    `db:"lobby_id" json:"lobby_id,omitempty"`
	Players    int    `db:"players" json:"players,omitempty"`
	Aggregate  int    `db:"aggregate" json:"aggregate,omitempty"`

	Pos    int    `db:"pos" json:"pos,omitempty"`
	Team   int    `db:"team" json:"team,omitempty"`
	Round  int    `db:"round" json:"round,omitempty"`
	Win    int    `db:"win" json:"win,omitempty"`
	Lose   int    `db:"lose" json:"lose,omitempty"`
	Kill   int    `db:"kill" json:"kill,omitempty"`
	Death  int    `db:"death" json:"death,omitempty"`
	Frame  int    `db:"frame" json:"frame,omitempty"`
	Result string `db:"result" json:"result,omitempty"`

	Created time.Time `db:"created" json:"created,omitempty"`
	Updated time.Time `db:"updated" json:"updated,omitempty"`
	System  uint32    `db:"system" json:"system,omitempty"`
}

type BattleCountResult struct {
	Battle int `json:"battle,omitempty"`
	Win    int `json:"win,omitempty"`
	Lose   int `json:"lose,omitempty"`
	Kill   int `json:"kill,omitempty"`
	Death  int `json:"death,omitempty"`
}

type RankingRecord struct {
	Rank int `db:"rank"`
	DBUser
}

type UserBan struct {
	Key     string    `db:"key" json:"key,omitempty"`
	Until   time.Time `db:"until" json:"until,omitempty"`
	Created time.Time `db:"created" json:"created,omitempty"`
}

type MLobbySetting struct {
	Platform         string `db:"platform" json:"platform"`
	Disk             string `db:"disk" json:"disk"`
	No               int    `db:"no" json:"no"`
	Name             string `db:"name" json:"name"`
	McsRegion        string `db:"mcs_region" json:"mcs_region"`
	Comment          string `db:"comment" json:"comment"`
	RuleID           string `db:"rule_id" json:"rule_id"`
	EnableForceStart bool   `db:"enable_force_start" json:"enable_force_start"`
	TeamShuffle      bool   `db:"team_shuffle" json:"team_shuffle"`
	PingLimit        bool   `db:"ping_limit" json:"ping_limit"`
}

type MRule struct {
	ID           string `db:"id" json:"id"`
	Difficulty   int    `db:"difficulty" json:"difficulty"`
	DamageLevel  int    `db:"damage_level" json:"damage_level"`
	Timer        int    `db:"timer" json:"timer"`
	TeamFlag     int    `db:"team_flag" json:"team_flag"`
	StageFlag    int    `db:"stage_flag" json:"stage_flag"`
	MsFlag       int    `db:"ms_flag" json:"ms_flag"`
	RenpoVital   int    `db:"renpo_vital" json:"renpo_vital"`
	ZeonVital    int    `db:"zeon_vital" json:"zeon_vital"`
	MaFlag       int    `db:"ma_flag" json:"ma_flag"`
	ReloadFlag   int    `db:"reload_flag" json:"reload_flag"`
	BoostKeep    int    `db:"boost_keep" json:"boost_keep"`
	RedarFlag    int    `db:"redar_flag" json:"redar_flag"`
	LockonFlag   int    `db:"lockon_flag" json:"lockon_flag"`
	Onematch     int    `db:"onematch" json:"onematch"`
	RenpoMaskPS2 int    `db:"renpo_mask_ps2" json:"renpo_mask_ps2"`
	ZeonMaskPS2  int    `db:"zeon_mask_ps2" json:"zeon_mask_ps2"`
	AutoRebattle int    `db:"auto_rebattle" json:"auto_rebattle"`
	NoRanking    int    `db:"no_ranking" json:"no_ranking"`
	CPUFlag      int    `db:"cpu_flag" json:"cpu_flag"`
	SelectLook   int    `db:"select_look" json:"select_look"`
	RenpoMaskDC  uint   `db:"renpo_mask_dc" json:"renpo_mask_dc"`
	ZeonMaskDC   uint   `db:"zeon_mask_dc" json:"zeon_mask_dc"`
	StageNo      int    `db:"stage_no" json:"stage_no"`
}

// DB is an interface of database operation.
type DB interface {
	// Init initializes the database.
	Init() error

	// Migrate converts old version database to current version.
	Migrate() error

	// RegisterAccount creates new user account.
	RegisterAccount(ip string) (*DBAccount, error)

	// RegisterAccountWithLoginKey creates new user account with specific login key.
	// This function enables userPeers to share login-key among different servers.
	RegisterAccountWithLoginKey(ip string, loginKey string) (*DBAccount, error)

	// GetAccountByLoginKey retrieves an account by login-key.
	GetAccountByLoginKey(key string) (*DBAccount, error)

	// GetAccountBySessionID retrieves an account by session-id.
	GetAccountBySessionID(sessionID string) (*DBAccount, error)

	// LoginAccount updates last login information and update sessionID.
	LoginAccount(account *DBAccount, sessionID string, ipAddr string, machineID string) error

	// RegisterUser creates new user.
	// An account can hold three userPeers.
	RegisterUser(loginKey string) (*DBUser, error)

	// GetUserList returns user list that the account holds.
	GetUserList(loginKey string) ([]*DBUser, error)

	// GetUser retrieves an account by user_id
	GetUser(userID string) (*DBUser, error)

	// LoginUser updates last login information.
	LoginUser(user *DBUser) error

	// UpdateUser updates all user's mutable information.
	UpdateUser(user *DBUser) error

	// AddBattleRecord saves new battle record.
	// This function is used when a battle starts.
	AddBattleRecord(battle *BattleRecord) error

	// GetBattleRecordUser load a battle record by battle_code and user_id.
	GetBattleRecordUser(battleCode string, userID string) (*BattleRecord, error)

	// UpdateBattleRecord updates all mutable information of battle_record.
	UpdateBattleRecord(record *BattleRecord) error

	// CalculateUserTotalBattleCount calculates battle count of the user.
	// You can get the results of one army using the `team` parameter.
	CalculateUserTotalBattleCount(userID string, team byte) (ret BattleCountResult, err error)

	// CalculateUserDailyBattleCount calculates daily battle count of the user.
	CalculateUserDailyBattleCount(userID string) (ret BattleCountResult, err error)

	// GetWinCountRanking returns top userPeers of win count.
	GetWinCountRanking(team byte) (ret []*RankingRecord, err error)

	// GetKillCountRanking returns top userPeers of kill count.
	GetKillCountRanking(team byte) (ret []*RankingRecord, err error)

	// GetString returns a string that corresponds to the key.
	GetString(key string) (value string, err error)

	// IsBannedEndpoint returns true if the endpoint is banned.
	IsBannedEndpoint(ip, machineID string) (banned bool, err error)

	// IsBannedAccount returns true if the account is banned.
	IsBannedAccount(account string) (banned bool, err error)

	// GetLobbySetting returns lobby setting.
	GetLobbySetting(platform, disk string, no int) (*MLobbySetting, error)

	// GetRule returns game rule.
	GetRule(id string) (*MRule, error)
}
