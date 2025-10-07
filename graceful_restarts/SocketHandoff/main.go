package main

// Package main implements a minimal-but-complete single-file Go program that demonstrates
// zero-downtime graceful restart without any external libraries, using classic FD handoff +
// a simple "I'm ready" pipe handshake.
//
// Features:
// - Listens on :8080 and replies with "hello world" + PID and a monotonically increasing request id.
// - Every Nth request (default 3) is slow (default 10s), printing a heartbeat every second to stdout
//   so you can watch an old process finish a long request while new process serves fresh ones.
// - On SIGHUP: parent forks/execs a new copy of itself, passing the listening socket via ExtraFiles,
//   plus a pipe FD the child writes to when it is "ready". Parent stops accepting only after ready.
// - On SIGTERM/SIGINT: graceful shutdown (stop accepting, drain active connections, then exit).
// - Uses http.Server.ConnState to track active connections accurately, and syscall.RawConn to show
//   how to inspect the underlying file descriptor.
//
// Note: When we Close() the listener the http.Serve goroutine returns with an
// "use of closed network connection" error. This is expected and safe to ignore.
// We explicitly check for it before logging to avoid confusion.
//
// Useful references (read alongside this code):
// - net/http Server & ConnState: https://pkg.go.dev/net/http#Server
// - net.FileListener (FD -> Listener): https://pkg.go.dev/net#FileListener
// - os/exec ExtraFiles (FD inheritance): https://pkg.go.dev/os/exec#Cmd
// - Listener.File() dup semantics: https://pkg.go.dev/net#TCPListener.File
// - Unix signals (SIGHUP/SIGTERM): man 7 signal (https://man7.org/linux/man-pages/man7/signal.7.html)
// - Nginx/HAProxy graceful patterns (background): nginx reload docs, HAProxy seamless reload articles
//
// Tested on Linux/macOS. Windows does not support Unix signals in the same way; consider other patterns there.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ansiColors holds ANSI escape codes for different colors.
var ansiColors = []string{"\033[31m", "\033[32m", "\033[33m", "\033[34m", "\033[35m", "\033[37m"}

// colorCode is the randomly selected color for this process's logs.
var colorCode string

var readyPipeFD int

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

// getenvInt retrieves an environment variable as int, falling back to def if unset or invalid.
func getenvInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// getenvDur retrieves an environment variable as seconds and returns a time.Duration, fallback def.
func getenvDur(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return def
}

// activeConns is the current number of active HTTP connections.
// reqSeq increments for each incoming request to produce unique request IDs.
// connTrack tracks active connections for draining.
var (
	activeConns int64
	reqSeq      uint64
	connTrack   = newConnTracker()
)

// connTracker tracks active connections by listening to http.Server.ConnState callbacks.
// It increments/decrements activeConns appropriately.
type connTracker struct {
	mu   sync.Mutex
	seen map[net.Conn]bool // whether this conn is currently counted as active
}

// newConnTracker constructs a new connection tracker.
func newConnTracker() *connTracker { return &connTracker{seen: make(map[net.Conn]bool)} }

// onState updates active connection count based on HTTP state changes.
func (t *connTracker) onState(c net.Conn, st http.ConnState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch st {
	case http.StateNew:
		// not counted yet; we'll count on Active
	case http.StateActive:
		if !t.seen[c] {
			t.seen[c] = true
			atomic.AddInt64(&activeConns, 1)
		}
	case http.StateIdle, http.StateHijacked, http.StateClosed:
		if t.seen[c] {
			delete(t.seen, c)
			atomic.AddInt64(&activeConns, -1)
		}
	}
}

// main is the entrypoint: it sets up the listener, HTTP server, and handles graceful restart/shutdown signals.
func main() {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(os.Getpid())))
	colorCode = ansiColors[rnd.Intn(len(ansiColors))]
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	currentProcessPID := os.Getpid()

	var newListner net.Listener
	var err error

	// Determine if we are starting a new process or inheriting a listener FD via graceful restart.
	if os.Getenv("GRACEFUL_RESTART") == "1" {
		// Child path: reconstruct the listener from an inherited FD (default 3).
		// The default number is 3 because that will be the first open file after ,fd0(stdin),fd1(stdout),fd2(stderr)
		fdNum := 3
		if v := strings.TrimSpace(os.Getenv("GRACEFUL_FD")); v != "" {
			if n, conv := strconv.Atoi(v); conv == nil {
				fdNum = n
			}
		}
		parentFDCopy := os.NewFile(uintptr(fdNum), "graceful-listener")
		if parentFDCopy == nil {
			log.Fatalf("[%d] failed to open inherited FD=%d", currentProcessPID, fdNum)
		}
		newListner, err = net.FileListener(parentFDCopy)
		if err != nil {
			log.Fatalf("[%d] net.FileListener: %v", currentProcessPID, err)
		}
		// Note: No need to Close f here; net.FileListener consumes it.
		logf("[%d] child reconstructed listener from FD=%d", currentProcessPID, fdNum)

		// Optional: scrub GRACEFUL_* env so this process, when upgraded later, starts with a clean slate.
		_ = os.Unsetenv("GRACEFUL_RESTART")
		_ = os.Unsetenv("GRACEFUL_FD")
	} else {
		// Parent path: bind a fresh TCP listener on :8080
		addr, _ := net.ResolveTCPAddr("tcp", ":8080")
		primaryTCPlistner, err2 := net.ListenTCP("tcp", addr)
		if err2 != nil {
			log.Fatalf("[%d] listen :8080: %v", currentProcessPID, err2)
		}
		newListner = primaryTCPlistner
		logf("[%d] parent listening on :8080", currentProcessPID)
	}

	// Demonstrate syscall.RawConn to introspect the underlying FD (educational)
	if tl, ok := newListner.(*net.TCPListener); ok {
		if rc, err := tl.SyscallConn(); err == nil {
			rc.Control(func(fd uintptr) {
				logf("[%d] listener raw fd=%d (via SyscallConn)", currentProcessPID, fd)
			})
		}
	}

	// HTTP server setup: configure slow/heartbeat behaviour.
	slowEveryN := getenvInt("SLOW_EVERY_N", 3)
	slowDuration := getenvDur("SLOW_SECS", 10*time.Second)
	heartbeat := getenvDur("HEARTBEAT_SECS", 1*time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Increment global request id.
		id := atomic.AddUint64(&reqSeq, 1)
		slow := slowEveryN > 0 && (id%uint64(slowEveryN) == 0)

		// Log basic request info
		logf("[%d] req=%d %s %s slow=%v", currentProcessPID, id, r.Method, r.URL.Path, slow)

		if slow {
			// Simulate long-running work with heartbeat logs.
			start := time.Now()
			ticker := time.NewTicker(heartbeat)
			defer ticker.Stop()
			deadline := time.NewTimer(slowDuration)
			defer deadline.Stop()
			for {
				select {
				case <-ticker.C:
					elapsed := time.Since(start).Truncate(time.Second)
					logf("[%d] req=%d heartbeat: %s elapsed", currentProcessPID, id, elapsed)
				case <-deadline.C:
					logf("[%d] req=%d slow work finished after %s", currentProcessPID, id, slowDuration)
					goto done
				}
			}
		}
		// fast path
		// fallthrough
	done:
		fmt.Fprintf(w, "hello world from pid=%d req=%d\n", currentProcessPID, id)
	})

	srv := &http.Server{
		Handler:   mux,
		ConnState: connTrack.onState, // track active connections for draining.
	}

	// Signal handling: SIGHUP (upgrade), SIGTERM/SIGINT (shutdown)
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)

	// Serve in a goroutine so we can coordinate signals.
	serveErr := make(chan error, 1)
	go func() {
		// http.Serve will return when ln is closed (e.g., during upgrade/shutdown)
		serveErr <- srv.Serve(newListner)
	}()

	logf("[%d] serving on :8080 (GRACEFUL_RESTART=%s)", currentProcessPID, os.Getenv("GRACEFUL_RESTART"))

	// If this is a child from a graceful restart, notify parent we're ready.
	if readyPipeFD != 0 {
		pipe := os.NewFile(uintptr(readyPipeFD), "ready-pipe")
		n, err := pipe.Write([]byte("ready\n"))
		if err != nil {
			logf("[%d] failed to write ready signal: %v", currentProcessPID, err)
		} else {
			logf("[%d] wrote %d bytes to ready pipe", currentProcessPID, n)
		}
		_ = pipe.Close()
	}

	for {
		select {
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGHUP:
				logPhase("Restart sequence started")
				logf("[%d] received SIGHUP: attempting graceful restart", currentProcessPID)
				attemptGracefulRestart(newListner)
				logPhase("Graceful sequence finished")
			case syscall.SIGTERM, syscall.SIGINT:
				logf("[%d] received %v: graceful shutdown", currentProcessPID, sig)
				shutdownAndExit(srv)
			}
		case err := <-serveErr:
			// Serve returned. If this happens while we still have active connections, wait for drain.
			if !errors.Is(err, http.ErrServerClosed) && err != nil {
				// Only log non-expected errors; "use of closed network connection" is normal.
				if !strings.Contains(err.Error(), "use of closed network connection") {
					logf("[%d] http.Serve error: %v", currentProcessPID, err)
				}
			}
			waitForDrainAndExit()
		}
	}

}

// attemptGracefulRestart execs a new copy of ourselves with FD inheritance + readiness pipe.
func attemptGracefulRestart(currentLn net.Listener) {
	pid := os.Getpid()

	// To pass the listener, we need a dup'd *os.File from it.
	tcpLn, ok := currentLn.(*net.TCPListener)
	if !ok {
		logf("[%d] listener is not *net.TCPListener; cannot gracefully restart", pid)
		return
	}
	lf, err := tcpLn.File() // dup of the underlying FD; safe to pass across exec
	if err != nil {
		logf("[%d] TCPListener.File: %v", pid, err)
		return
	}
	// Pipe for readiness handshake: parent holds read end; child gets write end as extra FD.
	r, w, err := os.Pipe()
	if err != nil {
		logf("[%d] os.Pipe: %v", pid, err)
		_ = lf.Close()
		return
	}

	// Exec the same binary (argv[0]) or override with NEW_BINARY_PATH if provided.
	bin := os.Getenv("NEW_BINARY_PATH")
	if strings.TrimSpace(bin) == "" {
		bin = os.Args[0]
	}
	cmd := exec.Command(bin)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"GRACEFUL_RESTART=1",
		"GRACEFUL_FD=3",   // first ExtraFile goes to fd=3
		"READY_PIPE_FD=4", // second ExtraFile goes to fd=4
	)
	cmd.ExtraFiles = []*os.File{lf, w}

	if err := cmd.Start(); err != nil {
		logf("[%d] failed to start child: %v (keeping old process)", pid, err)
		_ = lf.Close()
		_ = r.Close()
		_ = w.Close()
		return
	}
	// Parent no longer needs child's copy of write end; child inherited it.
	_ = w.Close()
	_ = lf.Close()

	logf("[%d] started child pid=%d; waiting for readiness signal", pid, cmd.Process.Pid)

	// Wait for readiness with a timeout, but keep serving if child fails.
	readyCh := make(chan struct{})
	go func() {
		defer close(readyCh)
		reader := bufio.NewReader(r)
		line, _ := reader.ReadString('\n')
		if strings.TrimSpace(line) != "" {
			logf("[%d] child pid=%d signaled ready: %q", pid, cmd.Process.Pid, strings.TrimSpace(line))
		}
	}()

	select {
	case <-readyCh:
		logf("[%d] child is ready; closing listener in parent and beginning drain", pid)
		_ = currentLn.Close()
		_ = r.Close()
	case <-time.After(10 * time.Second):
		logf("[%d] child did not signal ready in time; keeping old process active", pid)
		_ = r.Close()
	}

}

// shutdownAndExit stops accepting, gracefully shuts down server, waits for drain, then exits.
func shutdownAndExit(srv *http.Server) {
	pid := os.Getpid()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logf("[%d] Server.Shutdown error: %v", pid, err)
	}
	waitForDrainAndExit()
}

// waitForDrainAndExit waits for all active connections to finish, then exits.
func waitForDrainAndExit() {
	pid := os.Getpid()
	deadline := time.Now().Add(60 * time.Second)
	for {
		ac := atomic.LoadInt64(&activeConns)
		if ac == 0 {
			logf("[%d] all connections drained; exiting", pid)
			os.Exit(0)
		}
		if time.Now().After(deadline) {
			logf("[%d] drain timeout; force exiting with %d active connections", pid, ac)
			os.Exit(0)
		}
		logf("[%d] draining... active=%d", pid, ac)
		time.Sleep(1 * time.Second)
	}
}

// init runs early in the child process. After starting the server, notify parent via the inherited pipe FD.
// For simplicity we invoke this from init if GRACEFUL_RESTART is set.

func init() {
	// Only in the child after a graceful restart
	if os.Getenv("GRACEFUL_RESTART") == "1" {
		if fdStr := os.Getenv("READY_PIPE_FD"); fdStr != "" {
			if fd, err := strconv.Atoi(fdStr); err == nil {
				readyPipeFD = fd
			}
		}
	}
}
