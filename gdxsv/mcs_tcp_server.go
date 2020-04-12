package main

import (
	"net"
)

type McsTCPServer struct {
	logic *McsHub
}

func NewTCPServer(logic *McsHub) *McsTCPServer {
	return &McsTCPServer{
		logic: logic,
	}
}

func (s *McsTCPServer) ListenAndServe(addr string) error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return err
	}
	listner, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}
	for {
		conn, err := listner.AcceptTCP()
		if err != nil {
			continue
		}
		peer := NewTCPPeer(conn)
		go peer.Serve(s.logic)
	}
}
