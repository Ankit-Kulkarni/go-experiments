# idGen

## Overview
Copy of Elasticsearch's time-based ID generator for producing roughly ordered unique identifiers. The implementation packs a timestamp, sequence number, and munged MAC address into 15 bytes and emits a URL-safe Base64 string.

## Running
- `go run .` to see the current millisecond timestamp, its binary form, and an example identifier.
- Import `generator.go` in other projects and call `ESTimeBasedUUIDGenerator().NextID()` for thread-safe IDs.

## Notes
- `mac.go` hashes the first available hardware address; ensure the host provides one before first use.
- The generator guards against clock skew by forcing monotonicity when the sequence resets.
