//go:build sqlite_trace || trace

package main

import (
	"database/sql"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
)

const slowQueryThreshold = 100 * time.Millisecond

const sqliteDriverName = "sqlite3_with_trace"

func init() {
	sql.Register(sqliteDriverName, &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			return conn.SetTrace(&sqlite3.TraceConfig{
				Callback:        traceCallback,
				EventMask:       sqlite3.TraceStmt | sqlite3.TraceProfile,
				WantExpandedSQL: true,
			})
		},
	})
}

func traceCallback(info sqlite3.TraceInfo) int {
	switch info.EventCode {
	case sqlite3.TraceStmt:
		logger.Debug("sqlite3_trace",
			zap.String("sql", info.StmtOrTrigger),
			zap.String("expanded", info.ExpandedSQL),
		)
	case sqlite3.TraceProfile:
		dur := time.Duration(info.RunTimeNanosec) * time.Nanosecond
		if dur >= slowQueryThreshold {
			logger.Warn("slow query detected",
				zap.Duration("duration", dur),
				zap.Int64("nanosec", info.RunTimeNanosec),
			)
		}
	}
	return 0
}
