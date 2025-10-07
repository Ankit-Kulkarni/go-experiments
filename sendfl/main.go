package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"syscall"
	"time"
)

// Traditional copy using a buffer in user space
func transferWithBuffer(conn net.Conn, file *os.File, bufferSize int) (int64, error) {
	buffer := make([]byte, bufferSize)
	var totalWritten int64 = 0

	for {
		// Read from file into buffer (kernel → user space)
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return totalWritten, err
		}
		if n == 0 {
			break
		}

		// Write buffer to socket (user space → kernel)
		written, err := conn.Write(buffer[:n])
		if err != nil {
			return totalWritten, err
		}
		totalWritten += int64(written)
	}
	return totalWritten, nil
}

// Using sendfile system call
func transferWithSendFile(conn net.Conn, file *os.File, fileSize int64) (int64, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return 0, fmt.Errorf("not a TCP connection")
	}

	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("failed to get raw connection: %v", err)
	}

	var written int
	var sysErr error

	err = rawConn.Write(func(fd uintptr) bool {
		written, sysErr = syscall.Sendfile(int(fd), int(file.Fd()), nil, int(fileSize))
		return true
	})

	if err != nil {
		return int64(written), err
	}
	if sysErr != nil {
		return int64(written), sysErr
	}

	return int64(written), nil
}

// Benchmark structure to hold results
type BenchmarkResult struct {
	Method         string
	Duration       time.Duration
	BytesWritten   int64
	MemoryBefore   uint64
	MemoryAfter    uint64
	MemoryIncrease uint64
}

// Get current memory usage
func getMemoryUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

// Run benchmark for a transfer method
func runBenchmark(method string, transferFn func() (int64, error)) BenchmarkResult {
	runtime.GC()            // Run garbage collection before test
	time.Sleep(time.Second) // Let system stabilize

	memBefore := getMemoryUsage()
	startTime := time.Now()

	written, err := transferFn()
	if err != nil {
		log.Printf("Error in %s: %v", method, err)
	}

	duration := time.Since(startTime)
	memAfter := getMemoryUsage()

	return BenchmarkResult{
		Method:         method,
		Duration:       duration,
		BytesWritten:   written,
		MemoryBefore:   memBefore,
		MemoryAfter:    memAfter,
		MemoryIncrease: memAfter - memBefore,
	}
}

func main() {
	// Create a test file (100MB of random data)
	fileSize := int64(100 * 1024 * 20) // 100MB
	testFile := "testfile.dat"

	if err := createTestFile(testFile, fileSize); err != nil {
		log.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFile)

	// Run benchmarks multiple times
	bufferSizes := []int{4 * 1024, 8 * 1024, 32 * 1024, 64 * 1024} // Different buffer sizes to test
	results := make([][]BenchmarkResult, 0)

	for i := 0; i < 3; i++ { // Run 3 iterations
		fmt.Println("Running iteration ", i)
		iterationResults := make([]BenchmarkResult, 0)

		// Test traditional copy with different buffer sizes
		for _, bufSize := range bufferSizes {
			fmt.Println("Testing traditional copy for buffer size ", bufSize/1024, " KB")
			result := benchmarkTraditionalCopy(testFile, fileSize, bufSize)
			iterationResults = append(iterationResults, result)
		}

		// Test sendfile
		fmt.Println("Testing sendfile way")
		result := benchmarkSendFile(testFile, fileSize)
		iterationResults = append(iterationResults, result)

		results = append(results, iterationResults)
		time.Sleep(time.Second) // Cool down between iterations
	}

	// Print results
	printResults(results, bufferSizes)
}

func createTestFile(filename string, size int64) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write random data
	buffer := make([]byte, 1024*1024) // 1MB buffer
	fmt.Println("Start creating file with size ", size/1024/1024, " MB")
	remaining := size
	for remaining > 0 {
		writeSize := int64(len(buffer))
		if remaining < writeSize {
			writeSize = remaining
		}
		if _, err := file.Write(buffer[:writeSize]); err != nil {
			return err
		}
		remaining -= writeSize
	}
	fmt.Println("Created file ", filename)
	return nil
}

func benchmarkTraditionalCopy(filename string, fileSize int64, bufferSize int) BenchmarkResult {
	listener, client := createSocketPairV2()
	defer listener.Close()
	defer client.Close()

	file, _ := os.Open(filename)
	defer file.Close()

	methodName := fmt.Sprintf("Traditional (buffer: %dKB)", bufferSize/1024)
	return runBenchmark(methodName, func() (int64, error) {
		return transferWithBuffer(client, file, bufferSize)
	})
}

func benchmarkSendFile(filename string, fileSize int64) BenchmarkResult {
	listener, client := createSocketPairV2()
	defer listener.Close()
	defer client.Close()

	file, _ := os.Open(filename)
	defer file.Close()

	return runBenchmark("sendfile", func() (int64, error) {
		return transferWithSendFile(client, file, fileSize)
	})
}

func createSocketPair() (net.Conn, net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}

	connChan := make(chan net.Conn)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}
		connChan <- conn
	}()

	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		log.Fatal(err)
	}

	server := <-connChan
	return server, client
}

func createSocketPairV2() (net.Conn, net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	clientConn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		log.Fatal(err)
	}

	serverConn, err := listener.Accept()
	if err != nil {
		log.Fatal(err)
	}

	return serverConn, clientConn
}

func printResults(results [][]BenchmarkResult, bufferSizes []int) {
	fmt.Println("\nBenchmark Results (averaged over 3 runs):")
	fmt.Println("==========================================")
	fmt.Printf("%-25s | %-15s | %-20s | %-15s\n",
		"Method", "Duration", "Memory Increase", "Throughput")
	fmt.Println("--------------------------------------------------------------------")

	// Calculate averages
	methodResults := make(map[string]struct {
		avgDuration   time.Duration
		avgMemory     uint64
		avgThroughput float64
	})

	// Aggregate results
	for _, iteration := range results {
		for _, result := range iteration {
			avg := methodResults[result.Method]
			avg.avgDuration += result.Duration
			avg.avgMemory += result.MemoryIncrease
			avg.avgThroughput += float64(result.BytesWritten) / result.Duration.Seconds()
			methodResults[result.Method] = avg
		}
	}

	// Calculate and print averages
	iterations := float64(len(results))
	for method, avg := range methodResults {
		avgDuration := time.Duration(float64(avg.avgDuration) / iterations)
		avgMemory := avg.avgMemory / uint64(iterations)
		avgThroughput := avg.avgThroughput / iterations

		fmt.Printf("%-25s | %13v | %18d | %13.2f MB/s\n",
			method,
			avgDuration.Round(time.Millisecond),
			avgMemory,
			avgThroughput/1024/1024)
	}
}
