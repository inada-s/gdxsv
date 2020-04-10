package battle

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

var _ Peer = (*TCPPeer)(nil)

type TCPPeer struct {
	BasePeer

	sendMtx sync.Mutex
	conn    *net.TCPConn
	room    *Room
	seq     uint32
}

func NewTCPPeer(conn *net.TCPConn) *TCPPeer {
	return &TCPPeer{
		conn: conn,
		seq:  1,
	}
}

func (u *TCPPeer) Close() error {
	return u.conn.Close()
}

func (u *TCPPeer) Serve(logic *Logic) {
	glog.Infoln("[TCP]", u.Address(), "Serve Start")
	time.Sleep(2 * time.Second)
	defer glog.Infoln("[TCP]", u.Address(), "Serve End")
	// c.f. ReflectMsg
	// 6X := category?
	// 1031 := request connection ID
	// nn6XXXXX1031XXXXXXXXXXXXXXXX
	data, _ := hex.DecodeString("0e610022103166778899aabbccdd")
	u.AddSendData(data)
	u.readLoop(logic)
	if u.room != nil {
		u.room.Leave(u)
		u.room = nil
	}
	u.conn.Close()
}

func (u *TCPPeer) AddSendMessage(msg *proto.BattleMessage) {
	u.AddSendData(msg.GetBody())
}

func (u *TCPPeer) AddSendData(data []byte) {
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

func (u *TCPPeer) Address() string {
	return u.conn.RemoteAddr().String()
}

func (u *TCPPeer) readLoop(logic *Logic) {
	buf := make([]byte, 1024)
	inbuf := make([]byte, 0, 128)

	for {
		// u.conn.SetReadDeadline(time.Now().Add(time.Second * 10))
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
			u.room = logic.Join(u, sessionID)
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
