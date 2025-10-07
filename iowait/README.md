# iowait

## Overview
Stress test for reproducing heavy I/O wait conditions. It spawns 3,000 goroutines that serialize on a mutex, append to `mydir/myfile.txt`, read the whole file back, and sleep for 50 seconds.

## Running
- `go run .` to create `mydir/` and start the goroutines.
- Observe OS metrics (e.g., `iostat`, `pidstat`) while the workload runs; stop with Ctrl+C when finished.

## Notes
- `modifyFile2Wait` loops forever; lower `numGoroutines` or `sleepDuration` before running on constrained systems.
- The shared mutex keeps the file operations serialized so the goroutines block, surfacing wait states in profilers.
