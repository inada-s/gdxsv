package main

import (
	"bufio"
	"fmt"
	"net"
	"time"
)

func handleConnection(conn net.Conn) {
	fmt.Println("Handling new connection...")

	// Close connection when this function ends
	defer func() {
		fmt.Println("Closing connection...")
		conn.Close()
	}()

	timeoutDuration := 5 * time.Second
	bufReader := bufio.NewReader(conn)

	for {
		// Set a deadline for reading. Read operation will fail if no data
		// is received after deadline.
		conn.SetReadDeadline(time.Now().Add(timeoutDuration))

		// Read tokens delimited by newline
		bytes, err := bufReader.ReadBytes('\n')
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Printf("%s", bytes)

		// conn.Write(bytes)
		conn.Write([]byte("12345678901234567890123456789012345678901234567890\n"))
	}
}

func main() {
	// Start listening to port 8888 for TCP connection
	listener, err := net.Listen("tcp", ":8888")
	if err != nil {
		fmt.Println(err)
		return
	}

	defer func() {
		listener.Close()
		fmt.Println("Listener closed")
	}()

	for {
		// Get net.TCPConn object
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println(err)
			break
		}

		go handleConnection(conn)
	}
}
