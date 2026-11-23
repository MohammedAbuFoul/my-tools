package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	// Ensure an executable path argument is provided
	if len(os.Args) < 2 {
		fmt.Println("Usage: b <path-to-executable> [args...]")
		os.Exit(1)
	}

	// First argument (index 1) is the target program
	exePath := os.Args[1]

	// Collect any remaining arguments to pass to the target program
	var exeArgs []string
	if len(os.Args) > 2 {
		exeArgs = os.Args[2:]
	}

	// Prepare the command
	cmd := exec.Command(exePath, exeArgs...)

	// Connect standard outputs so you can see the app running
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Starting benchmark for: %s %v\n", exePath, exeArgs)
	fmt.Println("--------------------------------------------------")

	// Capture high-precision start time
	startTime := time.Now()

	// Execute and wait for the program to complete
	err := cmd.Run()

	// Calculate total elapsed execution duration
	elapsedTime := time.Since(startTime)

	fmt.Println("--------------------------------------------------")
	if err != nil {
		fmt.Printf("Program exited with error: %v\n", err)
	}

	// Print total bench time
	fmt.Printf("Total Execution Time: %v\n", elapsedTime)
}
