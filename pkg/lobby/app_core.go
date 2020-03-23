package lobby

// ===========================
// Login
// ===========================

func (a *App) OnOpen(p *AppPeer) {
	RequestServerHello(p)
}

func (a *App) OnClose(p *AppPeer) {
	delete(a.users, p.UserID)
}
