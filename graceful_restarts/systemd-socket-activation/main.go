package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/coreos/go-systemd/activation"
)

var ansiColors = []string{"\033[31m", "\033[32m", "\033[33m", "\033[34m", "\033[35m", "\033[37m"}

var (
	colorCode string
	reqCount  uint64
	slowDelay = 10 * time.Second
	pid       = os.Getpid()
)

// logf automatically prefixes the PID and adds color.
func logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf(colorCode+"[%d] %s\033[0m", pid, msg)
}

// logPhase prints a colored section banner with PID.
func logPhase(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf(colorCode+"[%d] ==================== %s ====================\033[0m", pid, msg)
}

func main() {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(pid)))
	colorCode = ansiColors[rnd.Intn(len(ansiColors))]
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	logPhase("Starting process")

	listeners, err := activation.Listeners()
	if err != nil {
		log.Fatalf("[%d] activation.Listeners error: %v", pid, err)
	}
	if len(listeners) == 0 {
		logf("No systemd sockets found, falling back to manual listener on :8080")
		appL, _ := net.Listen("tcp", ":8080")
		listeners = []net.Listener{appL}
	}

	for i, l := range listeners {
		if l == nil {
			logf("Listener %d is nil, skipping", i)
			continue
		}
		logf("Listener %d: %s", i, l.Addr())
		go serve(i, l)
	}

	select {}
}

func serve(idx int, l net.Listener) {
	logPhase(fmt.Sprintf("Server %d listening on %s", idx, l.Addr()))
	for {
		conn, err := l.Accept()
		if err != nil {
			logf("Accept error on %s: %v", l.Addr(), err)
			return
		}
		reqID := atomic.AddUint64(&reqCount, 1)
		logf("Accepted req=%d from %s on %s", reqID, conn.RemoteAddr(), l.Addr())
		go handleConn(reqID, conn)
	}
}

func randString() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 5)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func handleConn(reqID uint64, c net.Conn) {
	defer c.Close()

	logf("req=%d new interactive session from %s", reqID, c.RemoteAddr())

	scanner := bufio.NewScanner(c)
	cmdCount := 0 // per-session command counter

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		cmdCount++
		logf("req=%d got command #%d: %q", reqID, cmdCount, line)

		// exit/quit terminates session cleanly
		if line == "exit" || line == "quit" {
			logf("req=%d client requested to close connection", reqID)
			c.Write([]byte("goodbye 👋\n"))
			return
		}

		// slow every 3rd *command* (not connection)
		slow := cmdCount%3 == 0
		random := randString()

		if slow {
			logf("req=%d cmd=%d slow mode (10s simulated work)", reqID, cmdCount)
			// run slow work in a goroutine so reading continues
			go func(line, random string, cmdNum int) {
				start := time.Now()
				for i := 1; i <= 10; i++ {
					time.Sleep(1 * time.Second)
					elapsed := time.Since(start).Truncate(time.Second)
					logf("req=%d cmd=%d heartbeat: %v elapsed", reqID, cmdNum, elapsed)
				}
				logf("req=%d cmd=%d finished simulated work", reqID, cmdNum)
				c.Write([]byte(fmt.Sprintf("slow reply [%s]: %s\n", random, line)))
			}(line, random, cmdCount)
		} else {
			c.Write([]byte(fmt.Sprintf("fast reply [%s]: %s\n", random, line)))
		}
	}

	if err := scanner.Err(); err != nil {
		logf("req=%d scanner error: %v", reqID, err)
	}
	logf("req=%d connection closed", reqID)
}
