package model

import (
	"gdxsv/pkg/db"
)

const (
	EntryNone   = 0
	EntryAeug   = 1
	EntryTitans = 2
)

type User struct {
	db.User
	Entry byte
	Bin   string

	Room   *Room
	Lobby  *Lobby
	Battle *Battle
}
