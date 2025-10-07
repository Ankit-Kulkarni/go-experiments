package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
)

func main() {
	// Address to listen on
	listenAddr := "0.0.0.0:2525"

	// Address of the Milter service
	milterAddr := "127.0.0.1:1234"

	// Start the proxy
	log.Printf("Starting proxy on %s, forwarding to %s\n", listenAddr, milterAddr)
	if err := startProxy(listenAddr, milterAddr); err != nil {
		log.Fatalf("Error starting proxy: %v", err)
	}
}

func startProxy(listenAddr, milterAddr string) error {
	// Start a listener
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Printf("Listening on %s\n", listenAddr)

	for {
		// Accept incoming connections
		fmt.Println("waiting for a connection on ", listenAddr)
		clientConn, err := listener.Accept()
		fmt.Println("got a new connection from  ", clientConn.RemoteAddr(), " on ", listenAddr)
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}
		fmt.Println("will start goroutine 1")

		// Handle each connection in a separate goroutine
		go func() {
			defer clientConn.Close()
			log.Printf("Connection accepted from %s\n", clientConn.RemoteAddr())

			// Connect to the Milter service
			fmt.Println("going to dial for destination milter connection")
			milterConn, err := net.Dial("tcp", milterAddr)
			fmt.Println("dialed new connection in go routine 1")
			if err != nil {
				log.Printf("Failed to connect to Milter service: %v", err)
				return
			}
			fmt.Println("connection successful")
			defer milterConn.Close()

			log.Printf("Connected to Milter service at %s\n", milterAddr)

			// Start bi-directional data transfer
			go transferData(clientConn, milterConn, "client -> milter via proxy ")
			transferData(milterConn, clientConn, "milter --> client  via proxy ")
		}()
		fmt.Println("i have started go routine, now i will listen to connection again ")
	}
}

type Message struct {
	Code byte
	Data []byte
}

// ReadPacket reads incoming milter packet
func ReadPacket(sock net.Conn) (*Message, error) {
	// read packet length
	var length uint32
	if err := binary.Read(sock, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	// read packet data
	data := make([]byte, length)
	if _, err := io.ReadFull(sock, data); err != nil {
		return nil, err
	}

	// prepare response data
	message := Message{
		Code: data[0],
		Data: data[1:],
	}

	return &message, nil
}

// WritePacket sends a milter response packet to socket stream
func WritePacket(sock net.Conn, msg *Message) error {
	buffer := bufio.NewWriter(sock)

	// calculate and write response length
	length := uint32(len(msg.Data) + 1)
	if err := binary.Write(buffer, binary.BigEndian, length); err != nil {
		return err
	}

	// write response code
	if err := buffer.WriteByte(msg.Code); err != nil {
		return err
	}

	// write response data
	if _, err := buffer.Write(msg.Data); err != nil {
		return err
	}

	// flush data to network socket stream
	if err := buffer.Flush(); err != nil {
		return err
	}

	return nil
}

func transferData(src, dst net.Conn, direction string) {
	fmt.Println("in transfer data: ", direction, src.LocalAddr().String(), dst.LocalAddr().String())
	buf := make([]byte, 4096) // 4 KB buffer
	for {
		// Read from the source
		n, err := src.Read(buf)
		if err == io.EOF {
			fmt.Println("the connection is closed so bye bye ", src.LocalAddr(), direction)
			return
		}
		if err != nil {
			log.Printf("[%s] Error reading from source: %v", direction, err)
			return
		}

		// Log the data being transferred
		log.Printf("[%s] Data: %s", direction, string(buf[:n]))

		// Write to the destination
		if _, err := dst.Write(buf[:n]); err != nil {
			log.Printf("[%s] Error writing to destination: %v", direction, err)
			return
		}
	}
}
