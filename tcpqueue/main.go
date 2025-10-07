// The client concurrently requests the server 10 times and sends data to the server after the TCP connection is established.
package main

import (
	"context"
	"log"
	"net"
	"sync"
	"time"
)

var wg sync.WaitGroup

func establishConn(ctx context.Context, i int) {
	defer wg.Done()
	conn, err := net.DialTimeout("tcp", ":8888", time.Second*5)
	if err != nil {
		log.Printf("%d, dial error: %v", i, err)
		return
	}
	log.Printf("%d, dial success", i)
	_, err = conn.Write([]byte("hello world how are you"))
	if err != nil {
		log.Printf("%d, send error: %v", i, err)
		return
	}
	select {
	case <-ctx.Done():
		log.Printf("%d, dail close", i)
	}
}

func main() {
	server()
	// time.Sleep(time.Second * 1)
	// ctx, cancel := context.WithCancel(context.Background())
	// for i := 0; i < 5; i++ {
	// 	wg.Add(1)
	// 	go establishConn(ctx, i)
	// }

	// go func() {
	// 	sc := make(chan os.Signal, 1)
	// 	signal.Notify(sc, syscall.SIGINT)
	// 	select {
	// 	case <-sc:
	// 		cancel()
	// 	}
	// }()

	// wg.Wait()
	// log.Printf("client exit")
}
