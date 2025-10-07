# tcpqueue

## Overview
Experiment for observing TCP accept queue behavior. The server in `server.go` listens on `:8888` and deliberately sleeps, while the commented client code in `main.go` can open multiple connections in parallel to stress the backlog.

## Running
- `go run .` starts the sleeping server (`server()` is called from `main`).
- Uncomment the client loop, adjust the worker count, and re-run to simulate bursts of connection attempts.

## Notes
- Tune your kernel backlog with `sudo sysctl -w net.core.somaxconn=<value>` as suggested in the source comments.
- Because the server never accepts data, terminate with Ctrl+C when you finish observing queue depth.
