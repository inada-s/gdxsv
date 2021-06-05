package main

import (
	"context"
	"gdxsv/gdxsv/proto"
	"go.uber.org/zap"
	pb "google.golang.org/protobuf/proto"
	"net"
	"sync"
	"time"
)

type McsUDPServer struct {
	mcs  *Mcs
	conn *net.UDPConn

	mtx   sync.Mutex
	peers map[string]*McsUDPPeer
}

func NewUDPServer(mcs *Mcs) *McsUDPServer {
	return &McsUDPServer{
		mcs:   mcs,
		peers: map[string]*McsUDPPeer{},
	}
}

func (s *McsUDPServer) ListenAndServe(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}

	s.conn = conn

	err = s.conn.SetReadBuffer(16 * 1024 * 1024)
	if err != nil {
		return err
	}

	err = s.conn.SetWriteBuffer(16 * 1024 * 1024)
	if err != nil {
		return err
	}

	return s.readLoop()
}

func (s *McsUDPServer) readLoop() error {
	logger.Info("start udp server read loop")
	pkt := proto.GetPacket()
	buf := make([]byte, 4096)

	for {
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			logger.Error("ReadFromUDP", zap.Error(err))
			continue
		}
		if n == 0 {
			continue
		}

		pkt.Reset()
		if err := pb.Unmarshal(buf[:n], pkt); err != nil {
			logger.Error("pb.Unmarshal", zap.Error(err))
			continue
		}

		key := addr.String() + "#" + pkt.GetSessionId()
		s.mtx.Lock()
		peer, found := s.peers[key]
		s.mtx.Unlock()

		fin := false
		switch pkt.GetType() {
		case proto.MessageType_Ping:
			ts := pkt.PingData.Timestamp
			pkt.Reset()
			pkt.Type = proto.MessageType_Pong
			pkt.PongData = &proto.PongMessage{
				Timestamp: ts,
			}
			if data, err := pb.Marshal(pkt); err != nil {
				logger.Error("pb.Marshal", zap.Error(err))
			} else {
				_, err := s.conn.WriteToUDP(data, addr)
				if err != nil {
					logger.Error("WriteToUDP", zap.Error(err))
				}
			}
		case proto.MessageType_HelloServer:
			sessionID := pkt.GetSessionId()
			userID := ""
			ok := found

			if !found && sessionID != "" {
				peer := NewMcsUDPPeer(s.conn, addr)
				peer.room = s.mcs.Join(peer, sessionID)
				if peer.room != nil {
					peer.logger.Info("join udp peer", zap.Any("key", key))
					ok = true
					userID = peer.UserID()

					s.mtx.Lock()
					s.peers[key] = peer
					s.mtx.Unlock()

					go func(key string) {
						peer.Serve(s.mcs)
						if peer.room != nil {
							peer.room.Leave(peer)
						}
						peer.logger.Info("leave udp peer", zap.String("reason", peer.GetCloseReason()))
						s.mtx.Lock()
						delete(s.peers, key)
						s.mtx.Unlock()
					}(key)
				} else {
					peer.logger.Info("udp peer failed to join room", zap.String("addr", key))
				}
			}
			if found {
				userID = peer.UserID()
			}

			pkt.Reset()
			pkt.Type = proto.MessageType_HelloServer
			pkt.HelloServerData = &proto.HelloServerMessage{
				Ok:     ok,
				UserId: userID,
			}
			if data, err := pb.Marshal(pkt); err != nil {
				logger.Error("pb.Marshal", zap.Error(err))
			} else {
				_, err := s.conn.WriteToUDP(data, addr)
				if err != nil {
					logger.Error("WriteToUDP", zap.Error(err))
				}
			}
		case proto.MessageType_Battle:
			if found {
				peer.OnReceive(pkt)
			} else {
				logger.Error("battle data received but peer not found", zap.Any("pkt", pkt), zap.Any("key", key))
				fin = true
			}
		case proto.MessageType_Fin:
			if found {
				reason := "fin"
				if pkt.GetFinData() != nil {
					reason = pkt.GetFinData().GetDetail()
				}
				peer.SetCloseReason(reason)
				_ = peer.Close()
			}
		default:
			logger.Warn("received unexpected pkt type packet ", zap.Any("pkt", pkt), zap.Any("key", key))
			fin = true
		}

		if fin {
			pkt.Reset()
			pkt.Type = proto.MessageType_Fin
			pkt.FinData = &proto.FinMessage{
				Detail: "close",
			}
			if data, err := pb.Marshal(pkt); err != nil {
				logger.Error("pb.Marshal", zap.Error(err))
			} else {
				_, err := s.conn.WriteToUDP(data, addr)
				if err != nil {
					logger.Error("WriteToUDP", zap.Error(err))
				}
			}
		}
	}
}

var _ McsPeer = (*McsUDPPeer)(nil)

type McsUDPPeer struct {
	BaseMcsPeer

	room    *McsRoom
	addr    *net.UDPAddr
	conn    *net.UDPConn
	rudp    *proto.BattleBuffer
	filter  *proto.MessageFilter
	chFlush chan struct{}
	chRecv  chan struct{}

	readingMtx sync.Mutex
	reading    []*proto.BattleMessage
	reading2   []*proto.BattleMessage

	closeMtx  sync.Mutex
	closeFunc func()
}

func NewMcsUDPPeer(conn *net.UDPConn, addr *net.UDPAddr) *McsUDPPeer {
	u := &McsUDPPeer{
		addr:    addr,
		conn:    conn,
		chFlush: make(chan struct{}, 1),
		chRecv:  make(chan struct{}, 1),
		rudp:    proto.NewBattleBuffer(""),
		filter:  proto.NewMessageFilter([]string{""}),
	}
	u.logger = logger.With(
		zap.String("proto", "udp"),
		zap.String("addr", addr.String()),
	)
	return u
}

func (u *McsUDPPeer) Close() error {
	u.closeMtx.Lock()
	fn := u.closeFunc
	u.closeFunc = nil
	u.closeMtx.Unlock()

	if fn != nil {
		fn()
	}
	return nil
}

func (u *McsUDPPeer) SetUserID(id string) {
	u.userID = id
	u.rudp.SetID(id)
	u.filter.SetAcceptIDs([]string{id})
}

func (u *McsUDPPeer) Serve(mcs *Mcs) {
	u.logger.Info("McsUDPPeer.Serve start")
	defer u.logger.Info("McsUDPPeer.Serve end")

	ctx, cancel := context.WithCancel(context.Background())

	u.closeMtx.Lock()
	u.closeFunc = cancel
	u.closeMtx.Unlock()

	defer cancel()
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()
	lastRecv := time.Now()
	lastSend := time.Now()

	pbBuf := make([]byte, 0)
	pbm := pb.MarshalOptions{Deterministic: true}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			timeout := time.Since(lastRecv).Seconds() > 10.0
			if timeout {
				u.SetCloseReason("sv_recv_timeout")
				return
			}
			if time.Since(lastSend).Seconds() >= 0.016 {
				select {
				case u.chFlush <- struct{}{}:
				default:
				}
			}
		case <-u.chFlush:
			lastSend = time.Now()
			data, seq, ack := u.rudp.GetSendData()
			pkt := proto.GetPacket()
			pkt.Type = proto.MessageType_Battle
			pkt.BattleData = data
			pkt.Ack = ack
			pkt.Seq = seq
			var err error
			pbBuf, err = pbm.MarshalAppend(pbBuf[:0], pkt)
			proto.PutPacket(pkt)
			if err != nil {
				u.logger.Error("Marshal error", zap.Error(err))
				u.SetCloseReason("sv_marshal_error")
				return
			}
			_, err = u.conn.WriteTo(pbBuf, u.addr)
			if err != nil {
				u.logger.Error("WriteTo", zap.Error(err))
				// Should be returned ?
				// return
			}
		case <-u.chRecv:
			lastRecv = time.Now()
			u.readingMtx.Lock()
			u.reading, u.reading2 = u.reading2, u.reading
			u.readingMtx.Unlock()

			for _, msg := range u.reading2 {
				if u.room == nil {
					u.logger.Warn("No room user sent", zap.Any("msg", msg))
					u.SetCloseReason("sv_no_room")
					return
				} else if u.room.IsClosing() {
					u.SetCloseReason("sv_room_closing")
					return
				} else {
					u.room.SendMessage(u, msg)
				}
			}
			u.reading2 = u.reading2[:0]
		}
	}
}

func (u *McsUDPPeer) OnReceive(pkt *proto.Packet) {
	u.rudp.ApplySeqAck(pkt.GetSeq(), pkt.GetAck())

	hasNewMsg := false
	u.readingMtx.Lock()
	for _, msg := range pkt.GetBattleData() {
		if u.filter.Filter(msg) {
			u.reading = append(u.reading, msg)
			hasNewMsg = true
		}
	}
	u.readingMtx.Unlock()

	if hasNewMsg {
		select {
		case u.chRecv <- struct{}{}:
		default:
		}
	}
}

func (u *McsUDPPeer) Address() string {
	return u.addr.String()
}

func (u *McsUDPPeer) AddSendData(data []byte) {
	u.logger.Fatal("AddSendData called", zap.Binary("data", data))
}

func (u *McsUDPPeer) AddSendMessage(msg *proto.BattleMessage) {
	u.rudp.PushBattleMessage(msg)
	select {
	case u.chFlush <- struct{}{}:
	default:
	}
}
