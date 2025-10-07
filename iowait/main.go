package main

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"sync"
	"time"
)

const (
	numGoroutines = 3000
	sleepDuration = 50 * time.Second
)

var (
	mutex    sync.Mutex
	filePath = "mydir/myfile.txt"
)

func main() {
	// Create the directory if it doesn't exist
	if err := os.MkdirAll("mydir", os.ModePerm); err != nil {
		fmt.Printf("Error creating directory: %v\n", err)
		return
	}

	// Create the file with some random text
	createFile()

	// Create and start goroutines
	for i := 1; i <= numGoroutines; i++ {
		go modifyFile2Wait(i)
	}

	// Wait for goroutines to finish
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	wg.Wait()

	fmt.Println("All goroutines finished.")
}

func createFile() {
	file, err := os.Create(filePath)
	if err != nil {
		fmt.Printf("Error creating file: %v\n", err)
		return
	}
	defer file.Close()

	randomText := generateRandomText()
	_, err = file.WriteString(randomText)
	if err != nil {
		fmt.Printf("Error writing to file: %v\n", err)
	}
}

func generateRandomText() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, 1024)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func modifyFile(goroutineNumber int) {
	fmt.Println("waiting go routine ", goroutineNumber)
	mutex.Lock()
	defer mutex.Unlock()
	fmt.Println("go routine: ", goroutineNumber)

	// Append the goroutine number to the file
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		mutex.Unlock()
		return
	}
	_, err = file.WriteString(fmt.Sprintf("Goroutine %d\n", goroutineNumber))
	if err != nil {
		fmt.Printf("Error writing to file: %v\n", err)
	}
	file.Close()

	// Sleep for the specified duration
	time.Sleep(sleepDuration)

	mutex.Unlock()
}

func modifyFile2Wait(goroutineNumber int) {
	for {
		fmt.Println("waiting go routine ", goroutineNumber)
		mutex.Lock()

		fmt.Println("go routine: ", goroutineNumber)
		// Append the goroutine number to the file
		file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
		if err != nil {
			fmt.Printf("Error opening file: %v\n", err)
			mutex.Unlock()
			return
		}
		_, err = file.WriteString(fmt.Sprintf("Goroutine %d\n", goroutineNumber))
		if err != nil {
			fmt.Printf("Error writing to file: %v\n", err)
		}
		file.Close()

		// Simulate I/O wait: Read the file's contents
		_, err = ioutil.ReadFile(filePath)
		if err != nil {
			fmt.Printf("Error reading from file: %v\n", err)
		}

		// Sleep for the specified duration
		time.Sleep(sleepDuration)

		mutex.Unlock()
	}
}
