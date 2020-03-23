package db

import (
	"bytes"
	"fmt"
	"math/rand"
	"time"
)

var DefaultDB DB

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

func GenBattleCode() string {
	return fmt.Sprintf("%013d", time.Now().UnixNano()/1000000)
}

func randInt(min int, max int) int {
	rand.Seed(time.Now().UTC().UnixNano())
	return min + rand.Intn(max-min)
}

type Account struct {
	LoginKey   string    `db:"login_key" json:"login_key,omitempty"`
	SessionID  string    `db:"session_id" json:"session_id,omitempty"`
	LastUserID string    `db:"last_user_id" json:"last_user_id,omitempty"`
	Created    time.Time `db:"created" json:"created,omitempty"`
	CreatedIP  string    `db:"created_ip" json:"created_ip,omitempty"`
	LastLogin  time.Time `db:"last_login" json:"last_login,omitempty"`
	System     byte      `db:"system" json:"system,omitempty"`
}

type User struct {
	LoginKey  string `db:"login_key" json:"login_key,omitempty"`
	SessionID string `db:"session_id" json:"session_id,omitempty"`

	UserID string `db:"user_id" json:"user_id,omitempty"`
	Name   string `db:"name" json:"name,omitempty"`
	Team   string `db:"team" json:"team,omitempty"`

	BattleCount       int `db:"battle_count" json:"battle_count,omitempty"`
	WinCount          int `db:"win_count" json:"win_count,omitempty"`
	LoseCount         int `db:"lose_count" json:"lose_count,omitempty"`
	KillCount         int `db:"kill_count" json:"kill_count,omitempty"`
	DeathCount        int `db:"death_count" json:"death_count,omitempty"`
	AeugBattleCount   int `db:"aeug_battle_count" json:"aeug_battle_count,omitempty"`
	AeugWinCount      int `db:"aeug_win_count" json:"aeug_win_count,omitempty"`
	AeugLoseCount     int `db:"aeug_lose_count" json:"aeug_lose_count,omitempty"`
	AeugKillCount     int `db:"aeug_kill_count" json:"aeug_kill_count,omitempty"`
	AeugDeathCount    int `db:"aeug_death_count" json:"aeug_death_count,omitempty"`
	TitansBattleCount int `db:"titans_battle_count" json:"titans_battle_count,omitempty"`
	TitansWinCount    int `db:"titans_win_count" json:"titans_win_count,omitempty"`
	TitansLoseCount   int `db:"titans_lose_count" json:"titans_lose_count,omitempty"`
	TitansKillCount   int `db:"titans_kill_count" json:"titans_kill_count,omitempty"`
	TitansDeathCount  int `db:"titans_death_count" json:"titans_death_count,omitempty"`

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
	Players    int    `db:"players" json:"players,omitempty"`
	Aggregate  int    `db:"aggregate" json:"aggregate,omitempty"`

	Pos    int    `db:"pos" json:"pos,omitempty"`
	Side   int    `db:"side" json:"side,omitempty"`
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
	User
}

// DB is an interface of database operation.
type DB interface {
	// Init initializes the database.
	Init() error

	// Migrate converts old version database to current version.
	Migrate() error

	// RegisterAccount creates new user account.
	RegisterAccount(ip string) (*Account, error)

	// RegisterAccountWithLoginKey creates new user account with specific login key.
	// This function enables users to share login-key among different servers.
	RegisterAccountWithLoginKey(ip string, loginKey string) (*Account, error)

	// GetAccountByLoginKey retrieves an account by login-key.
	GetAccountByLoginKey(key string) (*Account, error)

	// LoginAccount updates last login information.
	LoginAccount(*Account) error

	// RegisterUser creates new user.
	// An account can hold three users.
	RegisterUser(loginKey string) (*User, error)

	// GetUserList returns user list that the account holds.
	GetUserList(loginKey string) ([]*User, error)

	// GetUser retrieves an account by user_id
	GetUser(userID string) (*User, error)

	// LoginUser updates last login information.
	LoginUser(user *User) error

	// UpdateUser updates all user's mutable information.
	UpdateUser(user *User) error

	// AddBattleRecord saves new battle record.
	// This function is used when a battle starts.
	AddBattleRecord(battle *BattleRecord) error

	// GetBattleRecordUser load a battle record by battle_code and user_id.
	GetBattleRecordUser(battleCode string, userID string) (*BattleRecord, error)

	// UpdateBattleRecord updates all mutable information of battle_record.
	UpdateBattleRecord(record *BattleRecord) error

	// CalculateUserTotalBattleCount calculates battle count of the user.
	// You can get the results of one army using the `side` parameter.
	CalculateUserTotalBattleCount(userID string, side byte) (ret BattleCountResult, err error)

	// CalculateUserDailyBattleCount calculates daily battle count of the user.
	CalculateUserDailyBattleCount(userID string) (ret BattleCountResult, err error)

	// GetWinCountRanking returns top users of win count.
	GetWinCountRanking(side byte) (ret []*RankingRecord, err error)

	// GetWinCountRanking returns top users of kill count.
	GetKillCountRanking(side byte) (ret []*RankingRecord, err error)
}
