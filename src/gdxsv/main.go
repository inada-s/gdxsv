package main

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	l, err := net.Listen("tcp4", ":3333")
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}

	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			os.Exit(1)
		}
		fmt.Println("Accept: ", conn.RemoteAddr())
		go handleRequest(conn)
	}
}

func handleRequest(conn net.Conn) {
	buf := make([]byte, 1024)
	m := NewServerQuestion(0x6103)
	conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	_, err := conn.Write(m.Serialize())
	if err != nil {
		fmt.Println("Error writing:", err.Error())
		return
	}
	for {
		n, err := conn.Read(buf)
		if err != nil {
			fmt.Println("Error reading:", err.Error())
			return
		}
		if n == 0 {
			fmt.Println("read 0")
			return
		}
		fmt.Println("Recv", n, "bytes")
		fmt.Printf("%s", hex.Dump(buf[:n]))
		buf = buf[0:]
	}
}
