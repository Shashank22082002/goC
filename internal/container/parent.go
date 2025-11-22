//go:build linux
// +build linux

package container

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"

	"goC/internal/cgroups"
	"goC/internal/network"

	"goC/internal/utils"
)

const cgroupName = "goC-test-1"
const memoryLimitMB = 100

func RunParent(args []string) error {

	// 1. Validate args
	if err := validateArgs(args); err != nil {
		return err
	}

	// 2. Create a pipe for synchronization
	// The child will block reading from this pipe until we close it
	// This ensures the child waits for us to finish network setup
	pipe, err := utils.NewSyncPipe()
	if err != nil {
		return err
	}
	defer pipe.Close()

	// 3. Generate network names
	vethName, peerName := generateNetworkNames()

	// 4. We call the "runChild" now..
	// Setting up the command
	cmd := setupChildCommand(args, pipe, peerName)

	// 5. Setup cgroups
	// cgroups are a Linux kernel feature that allows you to limit and isolate the resources that a group of processes can use.
	// cgroups are used to implement resource limits, accounting, and isolation in a system.

	cgroupPath, err := cgroups.Setup(cgroupName, memoryLimitMB)
	if err != nil {
		return fmt.Errorf("Error setting up cgroups: %v", err)
	}
	defer cgroups.Cleanup(cgroupPath)

	// 6. Run the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Error running child: %v", err)
	}

	pid := cmd.Process.Pid

	// 7. Setup the network
	if err := network.SetupHostSide(pid, vethName, peerName); err != nil {
		// We log the error but don't fail; container will just have no network
		fmt.Printf("[Host] Error setting up host network: %v\n", err)
	}

	// 8. Add the container process to the cgroup
	if err := cgroups.AddProcess(cgroupPath, pid); err != nil {
		return fmt.Errorf("Error adding process to cgroup: %v", err)
	}

	// 9. Signal the child that network setup is complete
	// Close the write end of the pipe - this unblocks the child's read
	pipe.SignalReady()

	// 11. Wait for the child to exit
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("Error waiting for child: %v", err)
	}

	return nil
}

func setupChildCommand(args []string, pipe *utils.SyncPipe, peerName string) *exec.Cmd {
	// /proc/self/exe is a magic link to our *own* binary
	cmd := exec.Command("/proc/self/exe", append([]string{"runChild"}, args...)...)

	// 1. Setup flags
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: unix.CLONE_NEWPID | // New PID namespace
			unix.CLONE_NEWNS | // New Mount namespace
			unix.CLONE_NEWUTS | // New Hostname namespace
			unix.CLONE_NEWNET, // New Network namespace
	}

	// 2. Connect Stdin/Stdout/Stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 3. Pass the read end of the pipe to the child as ExtraFiles
	// It will appear as file descriptor 3 in the child process
	cmd.ExtraFiles = []*os.File{pipe.GetReadFile()}

	// 4. Set the "secret handshake" and peer name envs
	// We need to make sure these overrides any passed through command envs
	envs := []string{
		"GOC_INTERNAL_REEXEC=true",
		"GOC_PEER_NAME=" + peerName,
	}

	cmd.Env = append(envs, os.Environ()...)
	return cmd
}

func validateArgs(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("Must provide a rootfs and a command")
	}
	return nil
}

func generateNetworkNames() (vethName, peerName string) {
	vethName = "veth" + strconv.Itoa(os.Getpid())
	peerName = "c" + vethName
	return vethName, peerName
}
