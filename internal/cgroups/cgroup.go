//go:build linux
// +build linux

package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// The path to the cgroup v2 unified hierarchy
const cgroupRoot = "/sys/fs/cgroup"

// Setup creates a new cgroup, sets the memory limit,
// and returns the path to the cgroup.
func Setup(cgroupName string, memoryLimitMB int) (string, error) {
	// Create our cgroup directory: /sys/fs/cgroup/goC/cgroupName
	cgroupPath := filepath.Join(cgroupRoot, "goC", cgroupName)
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create cgroup dir: %v", err)
	}

	// 1. Enable the "memory" controller for this cgroup
	// We must write "+memory" to cgroup.subtree_control
	// This "activates" the memory controller for all sub-cgroups.
	// We do this on the *parent* /sys/fs/cgroup/goC directory.
	parentPath := filepath.Dir(cgroupPath)
	controlFile := filepath.Join(parentPath, "cgroup.subtree_control")
	if err := os.WriteFile(controlFile, []byte("+memory"), 0644); err != nil {
		// This might fail if it's already enabled, which is often fine.
		// A more robust solution checks the error, but for now, we'll log it.
		fmt.Printf("Note: failed to enable memory controller (may be ok): %v\n", err)
	}

	// 2. Set the memory limit
	// We write the limit in bytes to memory.max
	memoryLimitBytes := strconv.Itoa(memoryLimitMB * 1024 * 1024)
	limitFile := filepath.Join(cgroupPath, "memory.max")
	if err := os.WriteFile(limitFile, []byte(memoryLimitBytes), 0644); err != nil {
		return "", fmt.Errorf("failed to write memory.max: %v", err)
	}

	return cgroupPath, nil
}

// AddProcess adds a PID to the cgroup.
func AddProcess(cgroupPath string, pid int) error {
	// 3. Add our container's PID to cgroup.procs
	procsFile := filepath.Join(cgroupPath, "cgroup.procs")
	pidStr := strconv.Itoa(pid)
	if err := os.WriteFile(procsFile, []byte(pidStr), 0644); err != nil {
		return fmt.Errorf("failed to write to cgroup.procs: %v", err)
	}
	return nil
}

// Cleanup removes the cgroup directory.
func Cleanup(cgroupPath string) error {
	// We use os.Remove to remove the directory.
	// This will only succeed if cgroup.procs is empty (which it will be
	// after the container process exits).
	if err := os.Remove(cgroupPath); err != nil {
		return fmt.Errorf("failed to remove cgroup dir: %v", err)
	}
	return nil
}
