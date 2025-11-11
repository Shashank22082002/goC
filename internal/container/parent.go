//go:build linux
// +build linux

package container

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func RunParent(args []string) error {

	// 1. Validate args
	if len(args) < 2 {
		return fmt.Errorf("Must provide a rootfs and a command")
	}

	// 2. We call the "runChild" now
	// /proc/self/exe is a magic link to our *own* binary
	cmd := exec.Command("/proc/self/exe", append([]string{"runChild"}, args...)...)

	// 3. Setup flags
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: unix.CLONE_NEWPID | // New PID namespace
			unix.CLONE_NEWNS | // New Mount namespace
			unix.CLONE_NEWUTS, // New Hostname namespace
	}

	// 4. Connect Stdin/Stdout/Stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 5. Set the "secret handshake"
	cmd.Env = append(os.Environ(), "GOC_INTERNAL_REEXEC=true")

	// 6. Run the command
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Error running child: %v", err)
	}

	return nil
}
