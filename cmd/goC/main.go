//go:build linux
// +build linux

package main

import (
	"fmt"
	"os"

	"goC/internal/container"
)
// main is the entry point for our goC CLI.
// It acts as a "router" to decide what to do.
func main() {

	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <command> [args...]\n", os.Args[0])
		os.Exit(1)
	}


	switch os.Args[1] {
	case "run":
		// Call the parent logic from our container package
		if err := container.RunParent(os.Args[2:]); err != nil {
			fmt.Printf("[Host] Error: %v\n", err)
			os.Exit(1)
		}
	case "runChild":
		// Call the child logic from our container packag
		if err := container.RunChild(os.Args[2:]); err != nil {
			fmt.Printf("[Container] Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
