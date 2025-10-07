# transparentProxy

## Overview
TCP proxy that forwards connections from `0.0.0.0:2525` to a milter listening on `127.0.0.1:1234`. Logs every chunk transferred and includes helpers for reading and writing length-prefixed milter packets when you want to inspect or mutate traffic.

## Running
- `go run .` to start the proxy.
- Point your SMTP client at port 2525; ensure the downstream milter is reachable on 1234.

## Notes
- Remove or redact the payload logging in `transferData` before using this with real traffic—messages are logged in plain text.
- `ReadPacket`/`WritePacket` show how to handle the milter framing should you need to intercept specific commands.
