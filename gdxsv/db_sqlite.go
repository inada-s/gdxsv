package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

type SQLiteDB struct {
	*sqlx.DB
	*SQLiteCache
}

type SQLiteCache struct {
	mtx          sync.Mutex
	rankingCache map[string][]*RankingRecord
}

func NewSQLiteCache() *SQLiteCache {
	return &SQLiteCache{
		rankingCache: map[string][]*RankingRecord{},
	}
}

func (c *SQLiteCache) deleteRankingCache() {
	c.mtx.Lock()
	c.rankingCache = map[string][]*RankingRecord{}
	c.mtx.Unlock()
}

const schema = `CREATE TABLE IF NOT EXISTS account
(
    login_key     text,
    session_id    text    default '',
    last_user_id  text    default '',
    created_ip    text    default '',
    last_login_ip text    default '',
    last_login_machine_id text default '',
    created       timestamp,
    last_login    timestamp,
    system        integer default 0,
    PRIMARY KEY (login_key)
);
CREATE TABLE IF NOT EXISTS user
(
    user_id            text,
    login_key          text,
    session_id         text    default '',
    name               text    default 'default',
    team               text    default '',
    battle_count       integer default 0,
    win_count          integer default 0,
    lose_count         integer default 0,
    kill_count         integer default 0,
    death_count        integer default 0,
    renpo_battle_count integer default 0,
    renpo_win_count    integer default 0,
    renpo_lose_count   integer default 0,
    renpo_kill_count   integer default 0,
    renpo_death_count  integer default 0,
    zeon_battle_count  integer default 0,
    zeon_win_count     integer default 0,
    zeon_lose_count    integer default 0,
    zeon_kill_count    integer default 0,
    zeon_death_count   integer default 0,
    daily_battle_count integer default 0,
    daily_win_count    integer default 0,
    daily_lose_count   integer default 0,
    created            timestamp,
    system             integer default 0,
    PRIMARY KEY (user_id, login_key)
);
CREATE TABLE IF NOT EXISTS battle_record
(
    battle_code text,
    user_id     text,
    user_name   text,
    pilot_name  text,
    disk        text,
    lobby_id    integer,
    players     integer default 0,
    aggregate   integer default 0,
    pos         integer default 0,
    team        integer default 0,
    round       integer default 0,
    win         integer default 0,
    lose        integer default 0,
    kill        integer default 0,
    death       integer default 0,
    frame       integer default 0,
    result      text    default '',
    replay_url  text    default '',
    created     timestamp,
    updated     timestamp,
    system      integer default 0,
    PRIMARY KEY (battle_code, user_id)
);
CREATE TABLE IF NOT EXISTS m_string
(
    key   text,
    value text,
    PRIMARY KEY (key)
);
CREATE TABLE IF NOT EXISTS m_ban
(
    key     text,
    until   timestamp,
    created timestamp,
    PRIMARY KEY (key)
);
CREATE TABLE IF NOT EXISTS m_lobby_setting
(
    platform           text,
    disk               text,
    no                 integer,
    name               text,
    mcs_region         text default '',
    comment            text default '',
    reminder           text default '',
    rule_id            text default '',
    enable_force_start integer not null,
    team_shuffle       integer not null,
    ping_limit         integer not null,
    ping_region        text default '',
    patch_names        text default '',
    win_rate_limit     integer default 0,
    min_client_version text default '',
    PRIMARY KEY (platform, disk, no)
);
CREATE TABLE IF NOT EXISTS m_rule
(
    id             text,
    difficulty     integer not null,
    damage_level   integer not null,
    timer          integer not null,
    team_flag      integer not null,
    stage_flag     integer not null,
    ms_flag        integer not null,
    renpo_vital    integer not null,
    zeon_vital     integer not null,
    ma_flag        integer not null,
    reload_flag    integer not null,
    boost_keep     integer not null,
    redar_flag     integer not null,
    lockon_flag    integer not null,
    onematch       integer not null,
    renpo_mask_ps2 integer not null,
    zeon_mask_ps2  integer not null,
    auto_rebattle  integer not null,
    no_ranking     integer not null,
    cpu_flag       integer not null,
    select_look    integer not null,
    renpo_mask_dc  integer not null,
    zeon_mask_dc   integer not null,
    stage_no       integer not null,
    PRIMARY KEY (id)
);
CREATE TABLE IF NOT EXISTS m_patch
(
    platform 	text not null,
    disk     	text not null,
    name     	text not null,
    write_once 	integer not null,
    codes 	 	text not null,
    PRIMARY KEY (platform, disk, name)
);
`

const indexes = `
CREATE INDEX IF NOT EXISTS ACCOUNT_LAST_LOGIN_IP ON account(last_login_ip);
CREATE INDEX IF NOT EXISTS ACCOUNT_LAST_LOGIN_MACHINE_ID ON account(last_login_machine_id);
CREATE INDEX IF NOT EXISTS BATTLE_RECORD_USER_ID ON battle_record(user_id);
CREATE INDEX IF NOT EXISTS BATTLE_RECORD_PLAYERS ON battle_record(players);
CREATE INDEX IF NOT EXISTS BATTLE_RECORD_REPLAY_URL ON battle_record(replay_url);
CREATE INDEX IF NOT EXISTS BATTLE_RECORD_CREATED ON battle_record(created);
CREATE INDEX IF NOT EXISTS BATTLE_RECORD_AGGREGATE ON battle_record(aggregate);
`

func (db SQLiteDB) Init() error {
	_, err := db.Exec(schema + indexes)
	return err
}

func (db SQLiteDB) Migrate() error {
	ctx := context.Background()
	tables := []string{
		"account", "user", "battle_record",
		"m_string", "m_ban", "m_lobby_setting", "m_rule",
	}

	// begin tx
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelDefault})
	if err != nil {
		return errors.Wrap(err, "Begin failed")
	}

	// create table if not exists
	_, err = tx.Exec(schema)
	if err != nil {
		_ = tx.Rollback()
		return errors.Wrap(err, "Begin failed")
	}

	// copy all tables
	for _, table := range tables {
		tmp := table + "_tmp"
		_, err = tx.Exec(`ALTER TABLE ` + table + ` RENAME TO ` + tmp)
		if err != nil {
			_ = tx.Rollback()
			return errors.Wrap(err, "ALTER TABLE failed")
		}
	}

	// create new table
	_, err = tx.Exec(schema + indexes)
	if err != nil {
		_ = tx.Rollback()
		return errors.Wrap(err, "failed to create new tables")
	}

	// copy old table into new table and drop old table
	// it works unless key name is changed
	for _, table := range tables {
		tmp := table + "_tmp"
		rows, err := tx.Query(`SELECT * FROM ` + tmp + ` LIMIT 1`)
		if err != nil {
			_ = tx.Rollback()
			return errors.Wrap(err, "SELECT failed")
		}

		columns, err := rows.Columns()
		if err != nil {
			_ = tx.Rollback()
			return errors.Wrap(err, "Columns() failed")
		}
		_ = rows.Close()

		_, err = tx.Exec(`INSERT INTO ` + table + `(` + strings.Join(columns, ",") + `) SELECT * FROM ` + tmp)
		if err != nil {
			if err.Error() == "table battle_record has no column named side" {
				// NOTE: A column renamed. battle_record.side -> battle_record.team
				for i := 0; i < len(columns); i++ {
					if columns[i] == "side" {
						columns[i] = "team"
					}
				}
				_, err = tx.Exec(`INSERT INTO ` + table + `(` + strings.Join(columns, ",") + `) SELECT * FROM ` + tmp)
				if err != nil {
					_ = tx.Rollback()
					return errors.Wrap(err, "2021-02 INSERT failed")
				}
			} else if err.Error() == "table account has no column named last_login_cpuid" {
				// NOTE: A column renamed. account.last_login_cpuid -> account.last_login_machine_id
				for i := 0; i < len(columns); i++ {
					if columns[i] == "last_login_cpuid" {
						columns[i] = "last_login_machine_id"
					}
				}
				_, err = tx.Exec(`INSERT INTO ` + table + `(` + strings.Join(columns, ",") + `) SELECT * FROM ` + tmp)
				if err != nil {
					_ = tx.Rollback()
					return errors.Wrap(err, "2021-06 INSERT failed")
				}
			} else {
				_ = tx.Rollback()
				return errors.Wrap(err, "INSERT failed")
			}
		}

		_, err = tx.Exec(`DROP TABLE ` + tmp)
		if err != nil {
			_ = tx.Rollback()
			return errors.Wrap(err, "DROP TABLE failed")
		}
	}

	if os.Getenv("MIGRATE_202305") != "" {
		tx.Exec("UPDATE battle_record SET disk = 'dc2' WHERE disk IS NULL")
		tx.Exec("UPDATE battle_record SET lobby_id = 0 WHERE lobby_id IS NULL")
		tx.Exec(`
UPDATE battle_record SET pos = (
	SELECT COUNT(*) + 1
	FROM battle_record AS b2
	WHERE b2.battle_code = battle_record.battle_code AND b2.created < battle_record.created
)
WHERE pos = 0`)
		tx.Exec(`
UPDATE battle_record
SET pilot_name = substr(pilot_name, 1, length(pilot_name) - length(''))
WHERE pilot_name like '%' || X'00';
`)
	}

	if conf.GCSBaseURL != "" {
		if f, err := os.Open("gdxsv_replays.txt"); err == nil {
			sc := bufio.NewScanner(f)
			sc.Split(bufio.ScanLines)
			for sc.Scan() {
				line := sc.Text()
				if strings.HasSuffix(line, ".pb") {
					battleCode := strings.TrimSuffix(filepath.Base(line), ".pb")
					if len(battleCode) == 13 {
						url := conf.GCSBaseURL + strings.TrimPrefix(line, "gs://gdxsv")
						tx.Exec("UPDATE battle_record SET replay_url = ? WHERE battle_code = ?", url, battleCode)
					}
				}
			}
			f.Close()
		}
	}

	return tx.Commit()
}

func (db SQLiteDB) RegisterAccount(ip string) (*DBAccount, error) {
	key := genLoginKey()
	now := time.Now()
	_, err := db.Exec(`
INSERT INTO account
	(login_key, created_ip, created, last_login, system)
VALUES
	(?, ?, ?, ?, ?)`, key, ip, now, now, 0)
	if err != nil {
		return nil, err
	}
	a := &DBAccount{
		LoginKey:  key,
		CreatedIP: ip,
	}
	return a, nil
}

func (db SQLiteDB) RegisterAccountWithLoginKey(ip string, loginKey string) (*DBAccount, error) {
	now := time.Now()
	_, err := db.Exec(`
INSERT INTO account
	(login_key, created_ip, created, last_login, system)
VALUES
	(?, ?, ?, ?, ?)`, loginKey, ip, now, now, 0)
	if err != nil {
		return nil, err
	}
	a := &DBAccount{
		LoginKey:  loginKey,
		CreatedIP: ip,
	}
	return a, nil
}

func (db SQLiteDB) GetAccountByLoginKey(key string) (*DBAccount, error) {
	a := &DBAccount{}
	err := db.QueryRowx("SELECT * FROM account WHERE login_key = ?", key).StructScan(a)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (db SQLiteDB) GetAccountBySessionID(sid string) (*DBAccount, error) {
	a := &DBAccount{}
	err := db.QueryRowx("SELECT * FROM account WHERE session_id = ?", sid).StructScan(a)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (db SQLiteDB) LoginAccount(a *DBAccount, sessionID string, ipAddr string, machineID string) error {
	now := time.Now()
	_, err := db.Exec(`
UPDATE
	account
SET
	session_id = ?,
    last_login_ip = ?,
    last_login_machine_id = ?,
	last_login = ?
WHERE
	login_key = ?`,
		sessionID,
		ipAddr,
		machineID,
		now,
		a.LoginKey)
	if err != nil {
		return err
	}
	a.LastLogin = now
	a.SessionID = sessionID
	a.LastLoginIP = ipAddr
	a.LastLoginMachineID = machineID
	return nil
}

func (db SQLiteDB) RegisterUser(loginKey string) (*DBUser, error) {
	userID := genUserID()
	now := time.Now()
	_, err := db.Exec(`INSERT INTO user (user_id, login_key, created) VALUES (?, ?, ?)`, userID, loginKey, now)
	if err != nil {
		return nil, err
	}
	u := &DBUser{
		LoginKey: loginKey,
		UserID:   userID,
		Created:  now,
	}
	return u, nil
}

func (db SQLiteDB) GetUserList(loginKey string) ([]*DBUser, error) {
	rows, err := db.Queryx(`SELECT * FROM user WHERE login_key = ?`, loginKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*DBUser
	for rows.Next() {
		u := new(DBUser)
		err = rows.StructScan(u)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (db SQLiteDB) GetUser(userID string) (*DBUser, error) {
	u := &DBUser{}
	err := db.Get(u, `SELECT * FROM user WHERE user_id = ?`, userID)
	return u, err
}

func (db SQLiteDB) LoginUser(user *DBUser) error {
	a, err := db.GetAccountByLoginKey(user.LoginKey)
	if err != nil {
		return err
	}
	a.LastUserID = user.UserID

	_, err = db.Exec(`UPDATE account SET last_user_id = ? WHERE login_key = ?`, a.LastUserID, a.LoginKey)
	if err != nil {
		return err
	}

	_, err = db.Exec(`UPDATE user SET session_id = ? WHERE user_id = ?`, user.SessionID, user.UserID)
	return err
}

func (db SQLiteDB) UpdateUser(user *DBUser) error {
	_, err := db.NamedExec(`
UPDATE user
SET
	name = :name,
	team = :team,
	battle_count = :battle_count,
	win_count = :win_count,
	lose_count = :lose_count,
	kill_count = :kill_count,
	death_count = :death_count,
	renpo_battle_count = :renpo_battle_count,
	renpo_win_count = :renpo_win_count,
	renpo_lose_count = :renpo_lose_count,
	renpo_kill_count = :renpo_kill_count,
	renpo_death_count = :renpo_death_count,
	zeon_battle_count = :zeon_battle_count,
	zeon_win_count = :zeon_win_count,
	zeon_lose_count = :zeon_lose_count,
	zeon_kill_count = :zeon_kill_count,
	zeon_death_count = :zeon_death_count,
	daily_battle_count = :daily_battle_count,
	daily_win_count = :daily_win_count,
	daily_lose_count = :daily_lose_count,
	system = :system
WHERE
	user_id = :user_id`, user)
	return err
}

func (db SQLiteDB) AddBattleRecord(battleRecord *BattleRecord) error {
	now := time.Now()
	battleRecord.Updated = now
	battleRecord.Created = now
	_, err := db.NamedExec(`
INSERT INTO battle_record
	(disk, battle_code, user_id, user_name, pilot_name, lobby_id, players, aggregate, pos, team, created, updated, system, replay_url)
VALUES
	(:disk, :battle_code, :user_id, :user_name, :pilot_name, :lobby_id, :players, :aggregate, :pos, :team, :created, :updated, :system, :replay_url)`,
		battleRecord)
	return err
}

func (db SQLiteDB) UpdateBattleRecord(battle *BattleRecord) error {
	battle.Updated = time.Now()
	_, err := db.NamedExec(`
UPDATE battle_record
SET
	round = :round,
	win = :win,
	lose = :lose,
	kill = :kill,
	death = :death,
	frame = :frame,
	result = :result,
	updated = :updated,
	system = :system,
	replay_url = :replay_url
WHERE
	battle_code = :battle_code AND user_id = :user_id`, battle)

	if err == nil && battle.Aggregate != 0 {
		// refresh rakning page
		db.deleteRankingCache()
	}
	return err
}

func (db SQLiteDB) GetBattleRecordUser(battleCode string, userID string) (*BattleRecord, error) {
	b := new(BattleRecord)
	err := db.Get(b, `SELECT * FROM battle_record WHERE battle_code = ? AND user_id = ?`, battleCode, userID)
	return b, err
}

func (db SQLiteDB) SetReplayURL(battleCode string, url string) error {
	_, err := db.Exec(`UPDATE battle_record SET replay_url = ? WHERE battle_code = ?`, url, battleCode)
	return err
}

func (db SQLiteDB) ResetDailyBattleCount() (err error) {
	_, err = db.Exec(`
UPDATE
	user
SET
	daily_battle_count = 0,
	daily_win_count = 0,
	daily_lose_count = 0`)
	return
}

func (db SQLiteDB) CalculateUserTotalBattleCount(userID string, team byte) (ret BattleCountResult, err error) {
	if team == 0 {
		r := db.QueryRow(`
			SELECT TOTAL(round), TOTAL(win), TOTAL(lose), TOTAL(kill), TOTAL(death) FROM battle_record
			WHERE user_id = ? AND aggregate <> 0 AND players = 4`, userID)
		err = r.Scan(&ret.Battle, &ret.Win, &ret.Lose, &ret.Kill, &ret.Death)
		return
	}
	r := db.QueryRow(`
		SELECT TOTAL(round), TOTAL(win), TOTAL(lose), TOTAL(kill), TOTAL(death) FROM battle_record
		WHERE user_id = ? AND aggregate <> 0 AND players = 4 AND team = ?`, userID, team)
	err = r.Scan(&ret.Battle, &ret.Win, &ret.Lose, &ret.Kill, &ret.Death)
	return
}

func (db SQLiteDB) CalculateUserDailyBattleCount(userID string) (ret BattleCountResult, err error) {
	r := db.QueryRow(`
		SELECT TOTAL(round), TOTAL(win), TOTAL(lose), TOTAL(kill), TOTAL(death) FROM battle_record
		WHERE user_id = ? AND aggregate <> 0 AND players = 4 AND created > ?`,
		userID, time.Now().AddDate(0, 0, -1))
	err = r.Scan(&ret.Battle, &ret.Win, &ret.Lose, &ret.Kill, &ret.Death)
	return
}

func (db SQLiteDB) GetWinCountRanking(team byte) ([]*RankingRecord, error) {
	cacheKey := fmt.Sprint("win", team)
	db.mtx.Lock()
	ranking, ok := db.rankingCache[cacheKey]
	db.mtx.Unlock()
	if ok {
		return ranking, nil
	}

	var rows *sqlx.Rows
	var err error

	target := "win_count"
	if team == 1 {
		target = "renpo_win_count"
	} else if team == 2 {
		target = "zeon_win_count"
	}

	rows, err = db.Queryx(`
		SELECT RANK() OVER(ORDER BY `+target+` DESC) as rank,
		user_id, name, team,
		battle_count, win_count, lose_count, kill_count, death_count,
		renpo_battle_count, renpo_win_count, renpo_lose_count, renpo_kill_count, renpo_death_count,
		zeon_battle_count, zeon_win_count, zeon_lose_count, zeon_kill_count, zeon_death_count
		FROM user ORDER BY rank LIMIT ?`, 100)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ranking = []*RankingRecord{}
	for rows.Next() {
		u := new(RankingRecord)
		err = rows.StructScan(u)
		if err != nil {
			return nil, err
		}
		if !utf8.ValidString(u.Name) {
			u.Name = "？"
		}
		if !utf8.ValidString(u.Team) {
			u.Team = "？"
		}
		ranking = append(ranking, u)
	}

	db.mtx.Lock()
	db.rankingCache[cacheKey] = ranking
	db.mtx.Unlock()

	return ranking, nil
}

func (db SQLiteDB) GetKillCountRanking(team byte) ([]*RankingRecord, error) {
	cacheKey := fmt.Sprint("kill", team)
	db.mtx.Lock()
	ranking, ok := db.rankingCache[cacheKey]
	db.mtx.Unlock()
	if ok {
		return ranking, nil
	}

	var rows *sqlx.Rows
	var err error

	target := "kill_count"
	if team == 1 {
		target = "renpo_kill_count"
	} else if team == 2 {
		target = "zeon_kill_count"
	}

	rows, err = db.Queryx(`
		SELECT RANK() OVER(ORDER BY `+target+` DESC) as rank,
		user_id, name, team,
		battle_count, win_count, lose_count, kill_count, death_count,
		renpo_battle_count, renpo_win_count, renpo_lose_count, renpo_kill_count, renpo_death_count,
		zeon_battle_count, zeon_win_count, zeon_lose_count, zeon_kill_count, zeon_death_count
		FROM user ORDER BY rank LIMIT ?`, 100)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ranking = []*RankingRecord{}
	for rows.Next() {
		u := new(RankingRecord)
		err = rows.StructScan(u)
		if err != nil {
			return nil, err
		}
		if !utf8.ValidString(u.Name) {
			u.Name = "？"
		}
		if !utf8.ValidString(u.Team) {
			u.Team = "？"
		}
		ranking = append(ranking, u)
	}

	db.mtx.Lock()
	db.rankingCache[cacheKey] = ranking
	db.mtx.Unlock()

	return ranking, nil
}

func (db SQLiteDB) GetString(key string) (string, error) {
	var value string
	err := db.QueryRowx(`SELECT value FROM m_string WHERE key = ? LIMIT 1`, key).Scan(&value)
	return value, err
}

func (db SQLiteDB) IsBannedEndpoint(ip, machineID string) (bool, error) {
	banned := 0
	err := db.QueryRowx(`SELECT 1 FROM account WHERE
		(last_login_ip = ? OR (last_login_machine_id <> "" AND last_login_machine_id = ?)) AND
		(login_key IN (SELECT login_key FROM user WHERE user_id IN (SELECT key FROM m_ban WHERE datetime() < until))) LIMIT 1`,
		ip, machineID).Scan(&banned)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return banned == 1, err
}

func (db SQLiteDB) IsBannedAccount(loginKey string) (bool, error) {
	banned := 0
	err := db.QueryRowx(`SELECT 1 FROM account WHERE
		login_key = ? AND
		(login_key IN (SELECT login_key FROM user WHERE user_id IN (SELECT key FROM m_ban WHERE datetime() < until))) LIMIT 1`,
		loginKey).Scan(&banned)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return banned == 1, err
}

func (db SQLiteDB) GetLobbySetting(platform, disk string, no int) (*MLobbySetting, error) {
	m := &MLobbySetting{}
	err := db.QueryRowx("SELECT * FROM m_lobby_setting WHERE platform = ? AND disk = ? AND no = ?", platform, disk, no).StructScan(m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (db SQLiteDB) GetRule(id string) (*MRule, error) {
	m := &MRule{}
	err := db.QueryRowx("SELECT * FROM m_rule WHERE id = ?", id).StructScan(m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (db SQLiteDB) GetPatch(platform, disk, name string) (*MPatch, error) {
	m := &MPatch{}
	err := db.QueryRowx("SELECT * FROM m_patch WHERE platform = ? AND disk = ? AND name = ?", platform, disk, name).StructScan(m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (db SQLiteDB) FindReplay(q *FindReplayQuery) ([]*FoundReplay, error) {
	order := "DESC"
	if q.Reverse {
		order = "ASC"
	}

	rows, err := db.NamedQuery(`
SELECT
  disk,
  battle_code,
  lobby_id,
  players,
  MAX(round) as round,
  GROUP_CONCAT(pos, '/') AS pos_list,
  GROUP_CONCAT(team, '/') AS team_list,
  GROUP_CONCAT(win, '/') AS win_list,
  GROUP_CONCAT(user_id, '/') AS user_id_list,
  GROUP_CONCAT(user_name, '/') AS user_name_list,
  GROUP_CONCAT(pilot_name, '/') AS pilot_name_list,
  created AS start_at,
  replay_url
FROM battle_record
WHERE battle_code IN (
  SELECT DISTINCT
    battle_code
  FROM
    battle_record
  WHERE
    replay_url != ''
    AND (disk = :disk OR :disk = '')
    AND (battle_code = :battle_code OR :battle_code = '')
    AND (user_id = user_id OR :user_id = '')
    AND (user_name = :user_name OR :user_name = '')
    AND (pilot_name = :pilot_name OR :pilot_name = '')
    AND (lobby_id = :lobby_id OR :lobby_id = -1)
    AND (players = :players OR :players = -1)
    AND (aggregate = :aggregate OR :aggregate = -1)
  ORDER BY created `+order+`
  LIMIT 100 OFFSET (:page) * 100)
GROUP BY battle_code
`, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type sqlRow struct {
		Disk          string    `db:"disk"`
		BattleCode    string    `db:"battle_code"`
		LobbyID       int       `db:"lobby_id"`
		Players       int       `db:"players"`
		Round         int       `db:"round"`
		PosList       string    `db:"pos_list"`
		TeamList      string    `db:"team_list"`
		WinList       string    `db:"win_list"`
		UserIDList    string    `db:"user_id_list"`
		UserNameList  string    `db:"user_name_list"`
		PilotNameList string    `db:"pilot_name_list"`
		StartAt       time.Time `db:"start_at"`
		ReplayURL     string    `db:"replay_url"`
	}

	var result []*FoundReplay
	var r sqlRow
	for rows.Next() {
		err = rows.StructScan(&r)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		n := r.Players

		replay := FoundReplay{}
		replay.Round = r.Round
		replay.Disk = r.Disk
		replay.StartAt = r.StartAt.Unix()
		replay.ReplayURL = r.ReplayURL

		posList := strings.SplitN(r.PosList, "/", n)
		teamList := strings.SplitN(r.TeamList, "/", n)
		winList := strings.SplitN(r.WinList, "/", n)
		userIDList := strings.SplitN(r.UserIDList, "/", n)
		userNameList := strings.SplitN(r.UserNameList, "/", n)
		pilotNameList := strings.SplitN(r.PilotNameList, "/", n)

		if len(teamList) != n ||
			len(winList) != n ||
			len(userIDList) != n ||
			len(userNameList) != n ||
			len(pilotNameList) != n {
			continue
		}

		for i := 0; i < n; i++ {
			team, err := strconv.Atoi(teamList[i])
			if err != nil {
				return nil, err
			}

			pos, err := strconv.Atoi(posList[i])
			if err != nil {
				return nil, err
			}

			if team == TeamRenpo && replay.RenpoWin == 0 {
				replay.RenpoWin, err = strconv.Atoi(winList[i])
				if err != nil {
					return nil, err
				}
			}

			if team == TeamZeon && replay.ZeonWin == 0 {
				replay.ZeonWin, err = strconv.Atoi(winList[i])
				if err != nil {
					return nil, err
				}
			}

			replay.Users = append(replay.Users, &ReplayUser{
				UserID:    userIDList[i],
				UserName:  userNameList[i],
				PilotName: pilotNameList[i],
				Team:      team,
				Pos:       pos,
			})
		}

		sort.Slice(replay.Users, func(i, j int) bool {
			return replay.Users[i].Pos < replay.Users[j].Pos
		})

		result = append(result, &replay)
	}

	return result, nil
}
