// server listens on the 8888 TCP port

package main

import (
	"log"
	"net"
	"time"
)

func server() {
	l, err := net.Listen("tcp", ":8888")
	if err != nil {
		log.Printf("failed to listen due to %v", err)
	}
	defer l.Close()
	log.Println("listen :8888 success")

	for {
		time.Sleep(time.Second * 100)
	}
}

// sudo sysctl -w net.core.somaxconn=5
