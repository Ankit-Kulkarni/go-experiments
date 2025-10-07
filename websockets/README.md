# websockets

## Overview
Gin server that upgrades `/ws` requests to a Gorilla WebSocket connection and emits a counter message every second—useful for testing clients that expect continuous updates.

## Running
- `go run .` to start the server on `:8080`.
- Connect with a client such as `wscat -c ws://localhost:8080/ws` to observe the heartbeat messages.

## Notes
- The bundled `webs` binary is an earlier build artifact; regenerate it with `go build` after modifying the code.
- `upgrader` currently relies on the default origin check; supply `CheckOrigin` when exposing this endpoint on the internet.
