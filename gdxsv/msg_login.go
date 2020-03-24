package main

import "github.com/golang/glog"

// Login Sequence

// client: 0035b5e0 lobby_act_00_01
func SendConnectionID(p *AppPeer) {
	m := NewServerQuestion(0x6101)
	m.Seq = 1
	w := m.Writer()
	w.WriteString("abc123")
	p.SendMessage(m)
}

var _ = register(0x6101, "ConnectionId", func(p *AppPeer, m *Message) {
	p.SendMessage(NewServerQuestion(0x6103))
})

var _ = register(0x6103, "0x6103", func(p *AppPeer, m *Message) {
	// not used
	glog.Infoln("0x6103")
})
var _ = register(0x6820, "RegurationHeader", func(p *AppPeer, m *Message) {
	glog.Infoln("RegurationHeader")

	// FIXME: I think it is wrong.
	a := NewServerAnswer(m)
	w := a.Writer()
	w.WriteString("header1")
	w.WriteString("header2")
	p.SendMessage(a)

	a = NewServerAnswer(m)
	a.Command = 0x6821
	w = a.Writer()
	w.WriteString("body1")
	w.WriteString("body2")
	p.SendMessage(a)

	a = NewServerAnswer(m)
	a.Command = 0x6822
	p.SendMessage(a)
})

var _ = register(0x6110, "LoginType", func(p *AppPeer, m *Message) {
	glog.Infoln("LoginType", m.Reader().Read8())

	// FIXME: I think it is wrong.
	a := NewServerAnswer(m)
	a.Command = 0x6111
	w := a.Writer()
	w.Write8(1) // number of user id
	w.WriteString("GDXSV_")
	w.WriteString("ハンドルネーム")
	p.SendMessage(a)
})
