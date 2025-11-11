# goC - Container Runtime Design

## Stage 1: Basic Container Isolation

### Overview

**Goal**: Implement `goC run <rootfs-path> <command>` to run a command in an isolated environment.

**Key Constraint**: You cannot change the namespaces of a running process. You can only create a new process inside new namespaces.

---

## Core Concept: "Run a Command in a Box"

The fundamental idea is to run a command (like `/bin/sh`) inside a "virtual box" that is isolated from the rest of your system.

When a user executes:
```bash
goC run ./alpine-fs /bin/sh
```

They are saying: *"Run `/bin/sh`, but trick it into thinking `./alpine-fs` is its entire world."*

### Isolation Features

The isolated environment provides:

- **Filesystem Isolation**: The `<rootfs-path>` becomes the new root (`/`). The process cannot see host files.
- **Process Isolation**: The command becomes PID 1. It cannot see other host processes (Chrome, Spotify, etc.).
- **Hostname Isolation**: Gets its own hostname (e.g., `goC-container`), independent of the host's name.

---

## Command Structure

```bash
goC run <rootfs-path> <command>
```

### 1. `goC run`

The main command that creates and starts a new container in one step.

### 2. `<rootfs-path>` - The Container Filesystem

**What it is**: A directory path on your host containing a complete Linux filesystem.

**Analogy**: Think of this as a "system disk" for your new "virtual computer."

**Contents**: Must include standard directories like `/bin`, `/lib`, `/etc`, `/proc`, etc.

**How to create one**: Export from Docker (don't create manually):

```bash
# Create a directory to hold the filesystem
mkdir ./alpine-fs

# Export an Alpine Linux filesystem
docker export $(docker create alpine) | tar -C ./alpine-fs -xf -
```

Now `./alpine-fs` contains a complete Alpine Linux filesystem ready to use as `<rootfs-path>`.

### 3. `<command>` - The Program to Run

**What it is**: The specific program to run inside the container.

**Important**: This path is relative to the new rootfs, not the host.

#### Example 1: Interactive Shell

```bash
goC run ./alpine-fs /bin/sh
```

- Runs the `sh` shell program
- Looks for it at `<rootfs-path>/bin/sh`
- Becomes PID 1 inside the container
- Container stops when you exit the shell

#### Example 2: Single Command

```bash
goC run ./alpine-fs /bin/ls /
```

- Runs a non-interactive command
- Lists contents of the container's root
- Container exits immediately after command completes

---

## Implementation Flow

When you run `goC run ./alpine-fs /bin/sh`, the program executes:

### Parent Process

1. Starts as the initial process
2. Creates a child process in new namespaces (PID, Mount, UTS)
3. Waits for the child process to exit

### Child Process (Inside the "Box")

1. **Set hostname** to `goC-container`
2. **Use `pivot_root`** to make `./alpine-fs` the new root (`/`)
3. **Mount `/proc`** so tools like `ps` work correctly
4. **Execute the command** using `syscall.Exec` to replace itself with `/bin/sh`

### Result

- A `sh` process running as PID 1 in an isolated environment
- The parent waits for the child (and its shell) to exit
- Clean separation between host and container

---

## Filesystem Setup: Making the Container's World

### Overview

**Goal**: Take a directory on your host (e.g., `./alpine-fs`) and make the child process believe it is its entire computer (its root `/` filesystem).

**Main Tool**: `syscall.PivotRoot` - the "magic wand" that swaps the old root (`/`) with a new one.

**Challenge**: `PivotRoot` is very picky and has strict rules. The steps below satisfy its requirements and ensure proper cleanup.

---

### Step-by-Step Breakdown

#### 1. Make Mount Namespace Private

```go
syscall.Mount("none", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, "")
```

**Concept**: Mount Propagation Control

**Analogy**: By default, new mount namespaces are like having a "magic mirror" to the host's filesystem. If you mount something inside the container (like `/proc`), it "propagates" or "leaks" out, and the host also sees that new mount.

**Why we do it**: `MS_PRIVATE` severs this link. It tells the kernel, "From now on, any mount or unmount I do inside this namespace is my business and should not affect the host." This is essential for isolation.

---

#### 2. Bind Mount the Rootfs to Itself

```go
syscall.Mount(rootfs, rootfs, "", syscall.MS_BIND, "")
```

**Concept**: Bind Mount Trick

**Analogy**: This is the weirdest rule. `PivotRoot` refuses to work on a plain directory. It says, "I can only swap mount points." A "mount point" is a directory where a filesystem is attached.

**Why we do it**: We "trick" the kernel. By mounting the rootfs onto itself, we "promote" it from a simple directory to an official "mount point." Now `PivotRoot` is happy to work with it.

---

#### 3. Create a Directory for the Old Root

```go
os.MkdirAll(filepath.Join(rootfs, oldRoot), 0700)
```

**Concept**: Creating a "Put-Away" Directory

**Analogy**: When you swap the new root (`./alpine-fs`) for the old root (the host's `/`), you have to put the old root somewhere. You can't just delete it.

**Why we do it**: We create a directory inside our new rootfs (e.g., `./alpine-fs/old_root`) that will serve as a temporary holding pen for the entire host filesystem after the swap.

---

#### 4. Perform the Pivot Root

```go
syscall.PivotRoot(rootfs, oldRootPath)
```

**Concept**: The "Great Swap"

**What it does**: This is the main event. It tells the kernel two things:

1. Make the `rootfs` (our `./alpine-fs` directory) the new root (`/`)
2. Take the old root (the host's filesystem) and move it to `oldRootPath` (`./alpine-fs/old_root`)

**Result**: The process's view of the world is "pivoted." What used to be `./alpine-fs` is now `/`. What used to be `/` is now `/old_root`.

---

#### 5. Change to the New Root Directory

```go
syscall.Chdir("/")
```

**Concept**: Changing the Working Directory

**Analogy**: After the swap, our process is still "standing" in the old directory where it was born (e.g., `/home/user/goC`). But that path doesn't exist in the new filesystem. The process is "floating in a void."

**Why we do it**: We must immediately `Chdir` (Change Directory) to a valid place inside the new filesystem. `Chdir("/")` is the safest bet, as it "grounds" the process at the new root.

---

#### 6. Mount the `/proc` Filesystem

```go
syscall.Mount("proc", "/proc", "proc", 0, "")
```

**Concept**: Mounting a Virtual Filesystem

**Analogy**: Our new `alpine-fs` has an empty `/proc` directory. This is just a folder. But `/proc` is supposed to be a special, virtual directory that the kernel creates in memory to show you running processes (it's how `ps` works).

**Why we do it**: We are telling the kernel: "Please activate this empty `/proc` directory. Mount your special 'proc' filesystem onto it." This brings `/proc` to life inside the container, but it will only show the container's own processes (because we're in a new PID namespace).

---

#### 7. Cleanup: Unmount and Remove Old Root

```go
syscall.Unmount("/"+oldRoot, syscall.MNT_DETACH)
os.RemoveAll("/" + oldRoot)
```

**Concept**: Cleanup and Security

**Analogy**: The "put-away" host filesystem is still mounted inside our container at `/old_root`. This is a massive security hole. The container could theoretically break out by accessing it.

**Why we do it**: We `Unmount` the old root to completely sever the last link to the host filesystem. Then we `RemoveAll` the empty `/old_root` directory. Now, the container is truly isolated. It has no way to see or access the host's file tree.

---

### Summary

The filesystem setup process transforms a simple directory into an isolated root filesystem:

1. **Isolate mount operations** from the host
2. **Convert directory to mount point** (satisfy `PivotRoot` requirements)
3. **Create temporary location** for old root
4. **Swap the roots** (the main operation)
5. **Fix working directory** to be valid in new filesystem
6. **Activate virtual filesystems** like `/proc`
7. **Remove all traces** of the host filesystem for security