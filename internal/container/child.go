//go:build linux
// +build linux

package container

import (
	"fmt"
	"os"
	"syscall"
)

func RunChild(args []string) error {
	// 1. Check that our program called this
	if os.Getenv("GOC_INTERNAL_REEXEC") != "true" || os.Getpid() != 1 {
		return fmt.Errorf("this is an internal command and not meant to be run directly")
	}

	// 2. Validate 'args'
	if len(args) < 2 {
		return fmt.Errorf("must provide a rootfs and a command")
	}

	rootfs := args[0]
	command := args[1]
	cmdArgs := args[2:]

	// 3. Set hostname
	if err := syscall.Sethostname([]byte("goC")); err != nil {
		return fmt.Errorf("Error setting hostname: %v", err)
	}

	// 4. Setup the filesystem
	if err := setupFileSystem(rootfs); err != nil {
		return fmt.Errorf("Error setting up filesystem: %v", err)
	}

	// 5. Exec the new command
	// syscall.Exec replaces this child process with the user's command
	// The path would be relative to the *new* rootfs now
	// IMPORTANT: argv[0] must be the command itself for programs like BusyBox to work
	argv := append([]string{command}, cmdArgs...)

	// Debug: print what we're about to exec
	fmt.Printf("[DEBUG] About to exec: command=%s, argv=%v\n", command, argv)

	if err := syscall.Exec(command, argv, os.Environ()); err != nil {
		return fmt.Errorf("Error execing: %v", err)
	}

	return nil
}
