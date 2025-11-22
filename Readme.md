# goC (goContainer)

goC is a mini-container runtime written in Go from scratch.

Feel free to contribute, and learn!

## Project Goal

The goal of this project is not to build a production-ready container runtime, but to learn the low-level Linux mechanisms that make containers possible. We will build a tool (like runc) that can run a command in an isolated environment using namespaces, cgroups, and a root filesystem.

## The Roadmap: Our 5 Stages

We will build goC in clear, distinct stages.

### Stage 1: The Core Runner (Our runc clone)

**Goal:** Run a command in a new, isolated filesystem and process tree.

**Command:** `goC run <rootfs-path> <command>`

**Key Concepts:**

- **Namespaces:** CLONE_NEWPID (PID), CLONE_NEWNS (Mount), CLONE_NEWUTS (Hostname).
- **Filesystem:** pivot_root (to change the root / directory).
- **Execution:** syscall.Exec (to replace our Go process with the user's command).

### Stage 2: Resource Limits (Cgroups)

**Goal:** Prevent the container from using all the host's resources.

**Command:** `goC run --memory 100m --cpu 0.5 ...`

**Key Concepts:**

- **Cgroups v1/v2:** Understanding the cgroup filesystem (/sys/fs/cgroup).
- **Programmatic Control:** Writing to files like memory.limit_in_bytes and cpu.max to set limits.
- **PID Assignment:** Adding the container's new PID to cgroup.procs.

### Stage 3: Network Isolation (The "Virtual Patch Cable")

**Goal:** Give the container its own private IP address and network.

**Command:** `goC run --network ...`

**Key Concepts:**

- **Namespace:** CLONE_NEWNET (Network).
- **Host-side Setup:** Creating a veth (virtual ethernet) pair.
- **Bridging:** Attaching one end of the veth pair to a Linux bridge (like cni0 or goC0).
- **Container-side Setup:** Moving the other end of the veth pair into the container's namespace and configuring it with an IP.

### Stage 4: Image & Bundle Management (The containerd model)

**Goal:** Move from a simple rootfs directory to a proper "container bundle" (like OCI).

**Command:** `goC create <id> <bundle-path>`

**Key Concepts:**

- **OCI Spec:** Reading a config.json that defines the container's settings.
- **Union Filesystems:** Using overlay2 to stack a writable layer on top of a read-only "image" layer.
- **State:** Saving the container's state (e.g., "created", "running", "stopped").

### Stage 5: The Daemon (The dockerd model)

**Goal:** Create a long-running daemon to manage containers and a separate CLI to talk to it.

**Command:** `goCd` (the daemon) and `goC-cli` (the client).

**Key Concepts:**

- **API:** Building a simple API (e.g., HTTP or gRPC) for the daemon.
- **Daemon<->Runtime:** The daemon will be responsible for calling our Stage 1-4 goC binary to do the actual low-level work.
- **CLI<->Daemon:** The new goC-cli will just be a simple client that sends API requests.

---

## The Project Structure (A Proper Go Layout)

A proper directory structure is key. We use a standard Go project layout that separates our **main entrypoint** (the CLI) from our **internal logic** (the container "engine"). This structure will also make **Stage 5** (creating a daemon) *much* easier.

Here is the proposed structure:

```text
goC/
├── cmd/
│   └── goC/
│       └── main.go         # The main CLI entrypoint. Its ONLY job is to parse args.
│
├── internal/
│   └── container/
│       ├── child.go        # All logic for the child process (inside the namespaces)
│       └── parent.go       # All logic for the parent process (creating the child)
│
├── go.mod                  # Our Go module file
└── README.md               # Our project plan
```

**Why this structure?**

- **cmd/goC/main.go:** This is our main application. Its only job is to be the "face" of the CLI. It will parse arguments (like run) and then call the actual logic functions from our internal/ package.
- **internal/:** This is a special Go directory. Code inside internal/ can only be imported by other code inside our goC project. This is perfect for our core logic.
- **internal/container/:** We'll create a new package named container. This is where all our "Stage 1" container magic will live. parent.go and child.go will be part of this container package.