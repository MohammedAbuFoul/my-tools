package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Result struct {
	Command  string
	Duration time.Duration
	Error    error
}

func runCommand(command string, ch chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()

	commandArgs := strings.Fields(command)

	exePath := commandArgs[0]

	exeArgs := commandArgs[1:]

	start := time.Now()

	cmd := exec.Command(exePath, exeArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()

	duration := time.Since(start)

	ch <- Result{
		Command:  command,
		Duration: duration,
		Error:    err,
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: program \"command1\" \"command2\" ...")
		return
	}

	commands := os.Args[1:]

	results := make(chan Result, len(commands))

	var wg sync.WaitGroup

	for _, command := range commands {
		wg.Add(1)
		go runCommand(command, results, &wg)
	}

	wg.Wait()
	close(results)

	fmt.Println("\nResults:")
	for result := range results {
		if result.Error != nil {
			fmt.Printf("%s\n  Time: %v\n  Error: %v\n\n",
				result.Command,
				result.Duration,
				result.Error)
		} else {
			fmt.Printf("%s\n  Time: %v\n\n",
				result.Command,
				result.Duration)
		}
	}
}
