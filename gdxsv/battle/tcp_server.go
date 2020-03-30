package battle

import (
	"net"
)

type TCPServer struct {
	logic *Logic
}

func NewTCPServer(logic *Logic) *TCPServer {
	return &TCPServer{
		logic: logic,
	}
}

func (s *TCPServer) ListenAndServe(addr string) error {
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
