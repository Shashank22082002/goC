//go:build linux
// +build linux

package container

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// setupFilesystem runs inside the new mount namespace.
// It pivots the root filesystem to the new rootfs.
func setupFileSystem(rootfs string) error {
	const oldRoot = "old_root"

	// 1. Make root private
	if err := syscall.Mount("none", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to make root private: %v", err)
	}

	// 2. Bind mount the new rootfs to itself
	if err := syscall.Mount(rootfs, rootfs, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("failed to bind mount rootfs: %v", err)
	}

	// 2.5. Copy host's DNS settings
	if err := copyResolvConf(rootfs); err != nil {
		// We can warn, but shouldn't fail the whole container
		fmt.Printf("[Container] Warning: failed to copy resolv.conf: %v\n", err)
	}

	// 3. Create a directory for the old root
	oldRootPath := rootfs + "/" + oldRoot
	if err := os.MkdirAll(oldRootPath, 0700); err != nil {
		return fmt.Errorf("failed to create old_root dir: %v", err)
	}

	// 4. PivotRoot
	if err := syscall.PivotRoot(rootfs, oldRootPath); err != nil {
		return fmt.Errorf("failed to pivot root: %v", err)
	}

	// 5. Chdir to the new root "/"
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("failed to chdir to new root: %v", err)
	}

	// === MOUNT VIRTUAL FILESYSTEMS ===
	// We are now at the new root.

	// 6. Mount /proc
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		return fmt.Errorf("failed to mount /proc: %v", err)
	}

	// 7. NEW: Mount /sys
	// `sysfs` is a virtual filesystem that provides kernel information.
	// Many modern tools need this.
	if err := syscall.Mount("sysfs", "/sys", "sysfs", 0, ""); err != nil {
		return fmt.Errorf("failed to mount /sys: %v", err)
	}

	// 8. NEW: Mount /dev
	// `devtmpfs` is a virtual filesystem that provides device nodes.
	// This is CRITICAL for /dev/null, /dev/tty, /dev/stdin, etc.
	// This should fix the "applet not found" error.
	if err := syscall.Mount("devtmpfs", "/dev", "devtmpfs", 0, ""); err != nil {
		return fmt.Errorf("failed to mount /dev: %v", err)
	}
	// ---
	// A more "correct" runtime would mount an empty "tmpfs"
	// and then create the minimal devices (null, tty, zero)
	// but mounting "devtmpfs" is a powerful and simple solution
	// that gets us 99% of the way there.
	// ---

	// 9. Unmount and rmdir the old_root
	if err := syscall.Unmount("/"+oldRoot, syscall.MNT_DETACH); err != nil {
		// This may fail if the host's root is busy, but it's not critical
		// for the container to run. We can log it but not fail.
		// fmt.Printf("Warning: failed to unmount old_root: %v\n", err)
	}
	if err := os.RemoveAll("/" + oldRoot); err != nil {
		// fmt.Printf("Warning: failed to remove old_root dir: %v\n", err)
	}

	return nil
}

// copyResolvConf copies the host's /etc/resolv.conf into the new rootfs
func createResolvConf(rootfs string) error {
	contResolvFile := filepath.Join(rootfs, "etc", "resolv.conf")

	// Ensure /etc directory exists (it should, but just in case)
	if err := os.MkdirAll(filepath.Dir(contResolvFile), 0755); err != nil {
		return fmt.Errorf("failed to create /etc dir in rootfs: %v", err)
	}

	// Create the content for the file
	// We use 8.8.8.8 (Google) and 1.1.1.1 (Cloudflare) as public DNS servers
	content := []byte("nameserver 8.8.8.8\nnameserver 1.1.1.1\n")

	// Write the file
	if err := os.WriteFile(contResolvFile, content, 0644); err != nil {
		return fmt.Errorf("failed to write container resolv.conf: %v", err)
	}

	return nil
}
