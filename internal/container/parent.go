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
)

const cgroupName = "goC-test-1"
const memoryLimitMB = 100

func RunParent(args []string) error {

	// 1. Validate args
	if len(args) < 2 {
		return fmt.Errorf("Must provide a rootfs and a command")
	}

	// 2. Create a pipe for synchronization
	// The child will block reading from this pipe until we close it
	// This ensures the child waits for us to finish network setup
	r, w, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %v", err)
	}
	defer r.Close() // Parent doesn't need the read end
	defer w.Close() // Ensure write end is closed even if we error out

	// 3. We call the "runChild" now
	// /proc/self/exe is a magic link to our *own* binary
	cmd := exec.Command("/proc/self/exe", append([]string{"runChild"}, args...)...)

	// 4. Setup flags
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: unix.CLONE_NEWPID | // New PID namespace
			unix.CLONE_NEWNS | // New Mount namespace
			unix.CLONE_NEWUTS | // New Hostname namespace
			unix.CLONE_NEWNET, // New Network namespace
	}

	// 5. Connect Stdin/Stdout/Stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 6. Pass the read end of the pipe to the child as ExtraFiles
	// It will appear as file descriptor 3 in the child process
	cmd.ExtraFiles = []*os.File{r}

	// 7. Set the "secret handshake"
	cmd.Env = append(os.Environ(), "GOC_INTERNAL_REEXEC=true")

	// 8. Create a unique veth name
	vethName := "veth" + strconv.Itoa(os.Getpid())
	peerName := "c" + vethName // "c" for container, We use parent PID for simplicity
	cmd.Env = append(cmd.Env, "GOC_PEER_NAME="+peerName)

	// 7. Setup cgroups
	cgroupPath, err := cgroups.Setup(cgroupName, memoryLimitMB)
	if err != nil {
		return fmt.Errorf("Error setting up cgroups: %v", err)
	}
	defer cgroups.Cleanup(cgroupPath)

	// 8. Run the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Error running child: %v", err)
	}

	pid := cmd.Process.Pid

	// 9. Setup the network
	if err := network.SetupHostSide(pid, vethName, peerName); err != nil {
		// We log the error but don't fail; container will just have no network
		fmt.Printf("[Host] Error setting up host network: %v\n", err)
	}

	// 10. Signal the child that network setup is complete
	// Close the write end of the pipe - this unblocks the child's read
	w.Close()

	// 11. Add the container process to the cgroup
	if err := cgroups.AddProcess(cgroupPath, pid); err != nil {
		return fmt.Errorf("Error adding process to cgroup: %v", err)
	}

	// 11. Wait for the child to exit
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("Error waiting for child: %v", err)
	}

	return nil
}
