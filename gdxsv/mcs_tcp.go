package main

import (
	"encoding/hex"
	"io"
	"net"
	"sync"
	"time"

	"github.com/golang/glog"
	pb "github.com/golang/protobuf/proto"

	"gdxsv/gdxsv/proto"
)

type McsTCPServer struct {
	mcs *Mcs
}

func NewTCPServer(mcs *Mcs) *McsTCPServer {
	return &McsTCPServer{mcs: mcs}
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
		go peer.Serve(s.mcs)
	}
}

var _ McsPeer = (*McsTCPPeer)(nil)

type McsTCPPeer struct {
	BaseMcsPeer

	sendMtx sync.Mutex
	conn    *net.TCPConn
	room    *McsRoom
	seq     uint32
}

func NewTCPPeer(conn *net.TCPConn) *McsTCPPeer {
	return &McsTCPPeer{
		conn: conn,
		seq:  1,
	}
}

func (u *McsTCPPeer) Close() error {
	return u.conn.Close()
}

func (u *McsTCPPeer) Serve(mcs *Mcs) {
	glog.Infoln("[TCP]", u.Address(), "Serve Start")
	time.Sleep(2 * time.Second)
	defer glog.Infoln("[TCP]", u.Address(), "Serve End")
	// c.f. ps2 symbol ReflectMsg
	// 6X := category?
	// 1031 := request connection ID
	// nn6XXXXX1031XXXXXXXXXXXXXXXX
	data, _ := hex.DecodeString("0e610022103166778899aabbccdd")
	u.AddSendData(data)
	u.readLoop(mcs)
	if u.room != nil {
		u.room.Leave(u)
		u.room = nil
	}
	u.conn.Close()
}

func (u *McsTCPPeer) AddSendMessage(msg *proto.BattleMessage) {
	u.AddSendData(msg.GetBody())
}

func (u *McsTCPPeer) AddSendData(data []byte) {
	u.sendMtx.Lock()
	defer u.sendMtx.Unlock()
	for sum := 0; sum < len(data); {
		n, err := u.conn.Write(data[sum:])
		if err != nil {
			glog.Errorf("%v write error: %v\n", u.Address(), err)
			break
		}
		sum += n
	}
}

func (u *McsTCPPeer) Address() string {
	return u.conn.RemoteAddr().String()
}

func (u *McsTCPPeer) readLoop(mcs *Mcs) {
	buf := make([]byte, 128)
	inbuf := make([]byte, 0, 128)

	for {
		u.conn.SetReadDeadline(time.Now().Add(time.Second * 10))
		n, err := u.conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				glog.Errorf("%v read error: %v\n", u.Address(), err)
			}
			return
		}
		if IsFinData(buf) {
			return
		}
		inbuf = append(inbuf, buf[:n]...)

		if u.room == nil {
			glog.Infoln("room nil: ", u.Address())
			if len(inbuf) < 20 {
				continue
			}
			// 14 30 00 00 00 08 99 88 00 ff ff ff 35 39 31 32 39 32 36 39
			sessionID := string(inbuf[12:20])
			inbuf = inbuf[:0]
			glog.Infoln("[TCP] SessionID", sessionID, err)
			u.room = mcs.Join(u, sessionID)
			if u.room == nil {
				glog.Infoln("failed to join room: ", u.UserID(), u.Address())
				u.conn.Close()
				break
			}
			glog.Infoln("join success", u.Address())
		} else {
			var tmp []byte
			for 0 < len(inbuf) {
				size := int(inbuf[0])
				if size <= len(inbuf) {
					tmp = append(tmp, inbuf[:size]...)
					inbuf = inbuf[size:]
				} else {
					break
				}
			}
			if 0 < len(tmp) {
				msg := proto.GetBattleMessage()
				msg.Body = tmp
				msg.UserId = pb.String(u.UserID())
				msg.Seq = pb.Uint32(u.seq)
				u.seq++
				u.room.SendMessage(u, msg)
			}
		}
	}
}
