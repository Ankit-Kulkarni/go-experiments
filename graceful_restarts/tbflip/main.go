package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudflare/tableflip"
)

var ansiColors = []string{"\033[31m", "\033[32m", "\033[33m", "\033[34m", "\033[35m", "\033[37m"}

// colorCode is the randomly selected color for this process's logs.
var colorCode string

// logf prints a formatted log message in the process color, automatically resetting after.
func logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf(colorCode + msg + "\033[0m")
}

// logPhase prints a colored separator line for important phases.
func logPhase(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Printf(colorCode + "==================== " + msg + " ====================\033[0m")
}

func main() {
	// pick random color per process
	rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(os.Getpid())))
	colorCode = ansiColors[rnd.Intn(len(ansiColors))]
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	pid := os.Getpid()
	logPhase("Starting process pid=%d", pid)

	upg, err := tableflip.New(tableflip.Options{})
	if err != nil {
		logf("[%d] tableflip.New error: %v", pid, err)
		os.Exit(1)
	}
	defer upg.Stop()

	// Upgrade signal loop (README-style): on SIGHUP, request an upgrade.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP)
		for range sig {
			logPhase("pid=%d received SIGHUP → Upgrade()", pid)
			if err := upg.Upgrade(); err != nil {
				logf("[%d] Upgrade error: %v", pid, err)
			}
		}
	}()

	// Listen must be called before Ready (README contract)
	ln, err := upg.Listen("tcp", ":8080")
	if err != nil {
		logf("[%d] upg.Listen error: %v", pid, err)
		os.Exit(1)
	}
	defer ln.Close()
	logPhase("HTTP server pid=%d listening on :8080", pid)

	// Handler with slow every 3rd request + heartbeats
	var count int
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		count++
		slow := count%3 == 0
		logf("[%d] accepted req=%d %s %s slow=%v", pid, count, r.Method, r.URL.Path, slow)

		if slow {
			for i := 1; i <= 10; i++ {
				logf("[%d] req=%d heartbeat %d", pid, count, i)
				time.Sleep(1 * time.Second)
			}
		}
		fmt.Fprintf(w, "hello world pid=%d req=%d slow=%v\n", pid, count, slow)
	})

	// Use a real http.Server so we can gracefully Shutdown on Exit
	srv := &http.Server{Handler: http.DefaultServeMux}
	go func() {
		logf("[%d] starting http.Serve loop", pid)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logf("[%d] http.Serve error: %v", pid, err)
		}
	}()

	// Child signals readiness; parent will stop accepting but keep serving existing requests
	if err := upg.Ready(); err != nil {
		logf("[%d] Ready error: %v", pid, err)
		os.Exit(1)
	}
	logPhase("pid=%d signaled Ready()", pid)

	// Wait until it's time for this process to wind down (child is up or SIGTERM)
	<-upg.Exit()
	logPhase("pid=%d received Exit() — graceful shutdown", pid)

	// Gracefully shutdown old server: finish in-flight, refuse new
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logf("[%d] Server.Shutdown error: %v", pid, err)
	}
	logPhase("pid=%d shutdown complete", pid)
}
