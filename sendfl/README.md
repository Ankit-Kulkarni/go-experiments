# sendfl

## Overview
Benchmark comparing buffered file-to-socket copies against the `sendfile` syscall. It builds a ~100 MB test file, creates local TCP socket pairs, and records duration, memory delta, and throughput for each strategy.

## Running
- `go run .` to build the test file, execute three benchmark iterations, and print the averaged table.
- Adjust `bufferSizes` or `fileSize` near the top of `main.go` to explore different payloads.

## Notes
- `transferWithSendFile` requires a TCP connection (`net.TCPConn`); the helper `createSocketPairV2` supplies one for local tests.
- The program deletes `testfile.dat` on success; add additional cleanup if you break out early or add new temp files.
