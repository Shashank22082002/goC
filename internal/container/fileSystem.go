//go:build linux
// +build linux

package container

import (
	"fmt"
	"os"
	"syscall"
)


// This runs inside the new mount propagation
// It pivots the root filesystem to the new rootfs
func setupFileSystem(rootfs string) error {
	const oldRoot = "old_root"

	// 1. Make root private to prevent mount propagation
	if err := syscall.Mount("none", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return err
	}

	// 2. Bind mount the new rootfs to itself.
	// This is a "trick" to make it a valid mount point for pivot_root.
	if err := syscall.Mount(rootfs, rootfs, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to bind mount rootfs: %v", err)
	}

	// 3. Create a directory for the old root
	oldRootPath := rootfs + "/" + oldRoot
	if err := os.MkdirAll(oldRootPath, 0700); err != nil {
		return fmt.Errorf("failed to create old_root dir: %v", err)
	}

	// 4. PivotRoot: swaps the old root "/" with the new root "rootfs"
	if err := syscall.PivotRoot(rootfs, oldRootPath); err != nil {
		return fmt.Errorf("failed to pivot root: %v", err)
	}

	// 5. Chdir to the new root "/"
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("failed to chdir to new root: %v", err)
	}

	// 6. Mount /proc (essential for `ps` to work)
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("failed to mount /proc: %v", err)
	}

	// 7. Unmount and rmdir the old_root
	if err := syscall.Unmount("/"+oldRoot, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("failed to unmount old_root: %v", err)
	}

	if err := os.RemoveAll("/" + oldRoot); err != nil {
		return fmt.Errorf("failed to remove old_root dir: %v", err)
	}

	return nil

}
