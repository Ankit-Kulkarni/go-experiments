//Refrence:  https://www.haproxy.org/download/1.8/doc/proxy-protocol.txt
// important points for ProxyProtocol
// V1: Implementation
// 1 line US-ASCII representation SENT IMMEDIATELY after connection establishment . Has to be first thing before any data sending
// a string identifying the protocol : "PROXY" ( \x50 \x52 \x4F \x58 \x59 ) Seeing this string indicates that this is version 1 of the protocol.
// FOR any string we write it using [string]. Exact keywords are writeten exactly
// Anything after UNKNOWN KEYWORD shoud be ignored
// ASCII:  PROXY<1SPACE><TCP4/TCP6/UNKNOWN><1SPACE><IPV4/IPV6><1SPACE><IPV4><1SPACE><TCPSOUREPORT><1SPACE><TCPDESTPORT><CRLF>
// HEX: [\x50 \x52 \x4F \x58 \x59][\x20][(\x54 \x43 \x50 \x34)|(\x54 \x43 \x50 \x36)|(\x55 \x4E \x4B \x4E \x4F \x57 \x4E )][\x20][SOURCE_IP][\x20][DEST_IP][\x20][SOURCE_PORT][\x20][DEST_PORT]
// EXAMPLES

//   - TCP/IPv4 :
//       "PROXY TCP4 255.255.255.255 255.255.255.255 65535 65535\r\n"
//        => 5 + 1 + 4 + 1 + 15 + 1 + 15 + 1 + 5 + 1 + 5 + 2 = 56 chars

//   - TCP/IPv6 :
//       "PROXY TCP6 ffff:f...f:ffff ffff:f...f:ffff 65535 65535\r\n"
//     => 5 + 1 + 4 + 1 + 39 + 1 + 39 + 1 + 5 + 1 + 5 + 2 = 104 chars

//   - unknown connection (short form) :
//       "PROXY UNKNOWN\r\n"
//     => 5 + 1 + 7 + 2 = 15 chars
// TOTAL 108 chars should be good enough for haproxy 1
// Assuming 1 byte for each char , 108 bytes at receiver end should be max buffer size
// CRLF is mandatory
// Receiver parsing:
//    - Wait for CRLF to come. IF first 107 char CRFL is not there , say invalid
//    - Abort any invalid sequence immediately which breaks the sequence

// BELOW implementation assumes role of a load balancer to whome some client connects and it has to create proxy protocol headers
// FOR BELOW impmenetation simplicity we consider we are always proxying IPV4

// 2.2. Binary header format (version 2) Creation
//  Starts with 12 bytes block    \x0D \x0A \x0D \x0A \x00 \x0D \x0A \x51 \x55 \x49 \x54 \x0A
// 13th byte: Protocol Version and Command .
// 		Highest 4 bit is \x2 fixed . Fixed at receiver end also
// 		Lowest 4 bits is command i.e \x0 i.e proxy server initiated connection , \x1: for relayed connection . Anything else drop it
// 14th byte: Transport Protocol and address family
// 		Highest 4 bit address family. 0x0 : AF_UNSPEC  / 0x1 : AF_INET / 0x2 : AF_INET6  / 0x3 : AF_UNIX
//      AF_UNSPEC: The sender should use this family when sending LOCAL commands or when dealing with unsupported protocol families
// 		AF_INET: IPV4 4 bytes
// 		AF_INET6: IPV6 16 bytes
// 		AF_UNIX: Unix socket 108 bytes
// 		Lowest 4 bits is protocol. 0x0 : UNSPEC  / 0x1 : STREAM / 0x2 : DGRAM
// 		UNSPEC: The sender should use this family when sending LOCAL commands or when dealing with unsupported protocol families
//      STREAM: Using TCP/unix stream etc
//		DGRAM: using udp

// 2.2. Binary header format (version 2) Parsing

// FOR AF_UNSPEC the receiver should ignore information
// 14th byte unknown family reject
// Only the UNSPEC protocol byte (\x00) is mandatory to implement on the receiver provided it fallsback
// 15th byte

package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

func createPPV1Header(srcIP net.IP, dstIP net.IP, srcPort, dstPort uint16) ([]byte, error) {
	// "PROXY TCP4 255.255.255.255 255.255.255.255 65535 65535\r\n"
	var header string
	var err error

	// assuming we are proxying IPV4 only
	header = fmt.Sprintf("%s %s %s %s %d %d\r\n", "PROXY", "TCP4", srcIP, dstIP, srcPort, dstPort)
	return []byte(header), err
}

func parsePPv1Header(header []byte) (string, net.IP, net.IP, uint16, uint16, error) {
	// Convert the header to a string
	headerStr := string(header)

	// Check that the header ends with \r\n
	if !strings.HasPrefix(headerStr, "PROXY") {
		return "", nil, nil, 0, 0, fmt.Errorf("Invalid PROXY PROTOCOL v1")
	}

	// Check that the header ends with \r\n
	if !strings.HasSuffix(headerStr, "\r\n") {
		return "", nil, nil, 0, 0, fmt.Errorf("Invalid PROXY PROTOCOL ENDING")
	}

	// Remove the trailing \r\n for further processing
	headerStr = strings.TrimSuffix(headerStr, "\r\n")

	// Split the header into parts
	parts := strings.Fields(headerStr)
	if len(parts) != 6 {
		return "", nil, nil, 0, 0, fmt.Errorf("INVALID HEADER LENGTH")
	}

	// Check the protocol
	protocol := strings.ToLower(parts[1])
	if protocol != "tcp4" && protocol != "tcp6" && protocol != "unknown" {
		return "", nil, nil, 0, 0, fmt.Errorf("protocol must be 'tcp4', 'tcp6', or 'unknown'")
	}

	// Parse IP addresses
	srcIP := net.ParseIP(parts[2])
	dstIP := net.ParseIP(parts[3])
	if srcIP == nil {
		return "", nil, nil, 0, 0, fmt.Errorf("Invalid source IP Address. Ignoring protocol")
	}
	if dstIP == nil {
		return "", nil, nil, 0, 0, fmt.Errorf("Invalid dest IP Address. Ignoring protocol")
	}

	// Parse ports
	var srcPort, dstPort uint16
	if _, err := fmt.Sscanf(parts[4], "%d", &srcPort); err != nil || srcPort > 65535 {
		return "", nil, nil, 0, 0, fmt.Errorf("invalid source port, must be between 0-65535")
	}
	if _, err := fmt.Sscanf(parts[5], "%d", &dstPort); err != nil || dstPort > 65535 {
		return "", nil, nil, 0, 0, fmt.Errorf("invalid destination port, must be between 0-65535")
	}

	return protocol, srcIP, dstIP, srcPort, dstPort, nil
}

func main() {
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

	}
}
