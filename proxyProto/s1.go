package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
)

const (
	ppv2HeaderSize = 12
	ipv4Length     = 4
	portLength     = 2
)

func createPPv2Header(srcIP net.IP, dstIP net.IP, srcPort, dstPort uint16) ([]byte, error) {
	// PPv2 header size: 12 bytes
	// IPv4 addresses: 4 bytes each (2 total = 8 bytes)
	// Ports: 2 bytes each (2 total = 4 bytes)
	header := make([]byte, 12+8+4) // 12 + 8 + 4 = 24 bytes

	// Header Signature
	copy(header[0:12], []byte{0x0D, 0x0A, 0x0A, 0x0A, 0x21, 0x50, 0x52, 0x4F, 0x58, 0x59, 0x20, 0x32})

	// Command and Protocol Family
	header[12] = 0x00 // Command: New connection
	header[13] = 0x01 // Protocol Family: IPv4

	// Length of address information
	header[14] = 0x00
	header[15] = 0x14 // 20 bytes total: 2 IPs (4 bytes each) + 2 ports (2 bytes each)

	// Source and Destination IPs
	copy(header[16:20], srcIP.To4()) // Source IP
	copy(header[20:24], dstIP.To4()) // Destination IP

	// Source and Destination Ports
	binary.BigEndian.PutUint16(header[24:26], srcPort) // Source Port
	binary.BigEndian.PutUint16(header[26:28], dstPort) // Destination Port

	return header, nil
}

func handleConnection(clientConn net.Conn, s2Address string) {
	defer clientConn.Close()

	// Define S2 address (replace with your actual S2 address)
	s2Conn, err := net.Dial("tcp", s2Address)
	if err != nil {
		fmt.Println("Error connecting to S2:", err)
		return
	}
	defer s2Conn.Close()

	// Create a Proxy Protocol header
	clientAddr := clientConn.RemoteAddr().(*net.TCPAddr)
	s2Addr := s2Conn.LocalAddr().(*net.TCPAddr)

	ppv2Header, err := createPPv2Header(clientAddr.IP, s2Addr.IP, uint16(clientAddr.Port), uint16(s2Addr.Port))
	if err != nil {
		fmt.Println("Error creating PPv2 header:", err)
		return
	}

	// Send the Proxy Protocol header to S2
	if _, err := s2Conn.Write(ppv2Header); err != nil {
		fmt.Println("Error sending PPv2 header:", err)
		return
	}

	// Relay data between client and S2
	go func() {
		io.Copy(s2Conn, clientConn)
	}()
	io.Copy(clientConn, s2Conn)
}

func dmain() {
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Println("S1 is listening on :8080")
	for {
		clientConn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}

		// Handle each connection in a separate goroutine
		go handleConnection(clientConn, "localhost:8081") // Replace with S2's address
	}
}
