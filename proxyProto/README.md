# proxyProto

## Overview
Playground for PROXY protocol experiments. `server1.go` shows how to accept connections and parse v1 headers, while `s1.go` builds a v2 header before relaying traffic to a backend (the compiled `s2` binary mimics that backend).

## Running
- `go run server1.go` to start a listener on `:8080`.
- Extend `main()` or `s1.go` to forward connections and prepend the appropriate PROXY header before handing them to `s2`.

## Notes
- `createPPV1Header`/`parsePPv1Header` document the ASCII framing expected by HAProxy-compatible peers.
- Update the header builders if you need IPv6 or UNIX socket support; the comments outline the byte layout for each family.
