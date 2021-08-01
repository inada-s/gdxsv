package main

import "expvar"

var (
	mcsMetrics      = expvar.NewMap("gdxsv-mcs")
	mcsConns        = new(expvar.Int)
	mcsMessageRecv  = new(expvar.Int)
	mcsMessageSent  = new(expvar.Int)
	mcsProcOver5Ms  = new(expvar.Int)
	mcsProcOver10Ms = new(expvar.Int)
	mcsProcOver15Ms = new(expvar.Int)
	mcsProcOver20Ms = new(expvar.Int)
	mcsProcMaxMs    = new(expvar.Int)
)

func init() {
	mcsMetrics.Set("conn", mcsConns)
	mcsMetrics.Set("msg-recv", mcsMessageRecv)
	mcsMetrics.Set("msg-sent", mcsMessageSent)
	mcsMetrics.Set("proc-5ms", mcsProcOver5Ms)
	mcsMetrics.Set("proc-10ms", mcsProcOver10Ms)
	mcsMetrics.Set("proc-15ms", mcsProcOver15Ms)
	mcsMetrics.Set("proc-20ms", mcsProcOver20Ms)
	mcsMetrics.Set("proc-maxms", mcsProcMaxMs)
}
