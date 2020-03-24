package main

// Login Sequence
func SendServerHello(p *AppPeer) {
	m := NewServerQuestion(0x6101)
	m.Seq = 1
	w := m.Writer()
	w.Write16(0x2837)
	p.SendMessage(m)
}

var _ = register(0x6101, "AnsServerHello", func(p *AppPeer, m *Message) {
	// TODO Implement
})
