# Docksmith Architecture & Implementation Details

## Overview

Docksmith is a Docker-like container system built from scratch in Go, demonstrating three core concepts:

1. **Build caching with content-addressing** - Deterministic layer identification via SHA-256
2. **Process isolation** - Container execution using OS primitives (chroot, namespaces)
3. **Layer-based filesystem assembly** - Immutable, stackable disk images

## Project Structure

```
main.go                 # CLI entry point
go.mod / go.sum        # Go module definition

parser/
  parser.go            # Docksmithfile parsing and validation

state/
  state.go             # Manifest, layer, and cache management

engine/
  engine.go            # Build orchestration and caching logic

runtime/
  runtime.go           # Container execution interface
  runtime_linux.go     # Linux-specific isolation (chroot, namespaces)
  runtime_other.go     # Stub for non-Linux systems

cli/
  cli.go               # Command handlers (build, images, run, rmi)

utils/
  tar.go               # Tar operations and deterministic packing

sample/
  Docksmithfile        # Example build configuration 
  app/
    app.py             # Sample application
    inputs/
      hello.txt        # Input files for COPY instruction
      info.txt

scripts/
  demo.sh              # Full demonstration script
  setup_base_images.sh # Base image initialization
  make_rootfs_tar.sh   # Utility for creating base images
```

## Component Details

### Parser (parser/parser.go)

Parses Docksmithfile with six instruction types:

```go
type Instruction struct {
  Type  string   // FROM, COPY, RUN, WORKDIR, ENV, CMD
  Args  []string // Parsed arguments
  Raw   string   // Original line
  Line  int      // For error reporting
}
```

**Validation rules:**
- All instructions are case-insensitive
- Unknown instructions fail with line number
- FROM must be first instruction
- CMD requires JSON array format: `CMD ["exec", "arg1"]`
- Each instruction type has specific argument count requirements

### State Management (state/state.go)

Manages persistent state in `~/.docksmith/`:

#### ImageManifest
```go
type ImageManifest struct {
  Name      string
  Tag       string
  Digest    string           // SHA-256 of manifest JSON
  Created   string           // RFC3339 timestamp
  Config    ImageConfig
  Layers    []LayerInfo
}
```

**Digest calculation:**
1. Create manifest with empty digest field
2. Marshal to JSON
3. Compute SHA-256
4. Write final JSON with digest to file

#### LayerInfo
```go
type LayerInfo struct {
  Digest    string  // SHA-256 of tar archive
  Size      int64   // Byte size of tar file
  CreatedBy string  // Instruction that created layer
}
```

#### Cache
- Key: Deterministic hash of instruction + config + file hashes
- Value: Layer digest
- Storage: One file per key in `cache/` directory
- Lifetime: Persistent across builds

### Build Engine (engine/engine.go)

Orchestrates the build process:

```go
func (be *BuildEngine) Build(imageName, imageTag string) (*BuildOutput, error)
```

**Process:**
1. Parse Docksmithfile
2. For each instruction:
   - Calculate cache key
   - Check cache hit/miss
   - Execute if miss
   - Create layer tar
   - Extract layer into working rootfs
3. Create and save final manifest

**Cache key computation:**
```
SHA-256(
  instruction_text +
  current_workdir +
  sorted_env_vars +
  [for COPY: hash of source files]
)
```

**Layer creation:**
- Tar entries sorted lexicographically (determinism)
- Timestamps zeroed to 1970 (determinism)
- Only new/modified files included in delta
- Stored with SHA-256 digest as filename

### Runtime (runtime/runtime.go, runtime_linux.go)

Container execution with OS-level isolation:

**Linux implementation (runtime_linux.go):**
```go
cmd.SysProcAttr = &syscall.SysProcAttr{
  Chroot:     rootFS,                    // Change root filesystem
  Cloneflags: CLONE_NEWPID |             // New PID namespace
              CLONE_NEWUTS |             // New UTS namespace  
              CLONE_NEWIPC,              // New IPC namespace
}
```

**Process:**
1. Validate root privilege (required for chroot)
2. Create temp directory
3. Extract all layers in order
4. Apply chroot to temp directory
5. Set working directory, environment, command
6. Execute: `exec <cmd>` in isolated context
7. Cleanup temp directory

**Non-Linux:** Stub function (user must be on Linux for full functionality)

### CLI (cli/cli.go)

Command handlers with argument parsing:

- **build**: Orchestrate compilation
- **images**: List all built images
- **run**: Execute container from image
- **rmi**: Remove image manifest and optionally layers

## Technical Decisions

### Deterministic Builds

**Problem:** Same source code should produce identical digests.

**Solution:**
1. **Tar entry ordering** - Sort all entries by path before adding
2. **Timestamp zeroing** - Set all timestamps to Unix epoch (1970)
3. **Sorted environment variables** - Lexicographic order for consistent hashing
4. **JSON formatting** - Fixed indentation (2 spaces)

### Cache Invalidation (Cascade)

Once a cache miss occurs, all subsequent instructions miss.

**Rationale:** Changes to earlier layers affect all dependent layers. Alternative: track which layers changed (requires more state).

### No Reference Counting

Deleting one image may break others sharing layers.

**Rationale:** Simplicity. Production systems track reference counts.

### Linux-Only Isolation

Process isolation via chroot + namespaces requires Linux.

**MacOS:** Cannot use chroot or namespaces → containers run on host filesystem. Other systems: complete stub.

**Why not containerD/runc?**  
Goal is educational - understanding isolation primitives. Using existing runtimes would hide the mechanism.

## Storage Layout

```
~/.docksmith/
├── images/
│   ├── ubuntu_20.04.json      # Image manifest
│   └── myapp_latest.json
├── layers/
│   ├── a1b2c3d4...            # SHA-256 digest
│   ├── e5f6g7h8...
│   └── ...
└── cache/
    ├── abc123def456...        # Cache key
    └── ...                     # Contains: layer digest as content
```

## Docksmithfile Semantics

### FROM
- Loads base image by name:tag
- Fails if image doesn't exist locally
- Extracts all base layers into working filesystem

### COPY
- Supports glob patterns: `*` (single-level), `**` (multi-level)
- Creates destination directories if missing
- Source files copied with original permissions
- Creates new layer

### RUN
- Executes shell command: `/bin/sh -c "command"`
- Command runs inside container with chroot
- Working directory and environment from image config
- Creates new layer

### WORKDIR
- Sets working directory for subsequent instructions
- Creates path if missing
- Does NOT create layer (metadata-only)

### ENV
- Sets environment variable in image config
- Format: `KEY=VALUE`
- Injected into containers and subsequent RUN commands
- Does NOT create layer (metadata-only)

### CMD
- Sets default container command
- Must be JSON array: `CMD ["executable", "arg1", "arg2"]`
- Used when `docksmith run image:tag` called without command
- Does NOT create layer (metadata-only)

## Key Algorithms

### Layer Creation (CreateLayerTar)
1. Collect all source files from glob patterns
2. Sort by path (determinism)
3. For each file:
   - Add tar header with zeroed timestamp
   - Copy file content
4. Compute SHA-256 while writing
5. Return digest and size

### Cache Key Computation
1. Hash instruction text
2. Hash current working directory
3. For each env var (sorted by key):
   - Hash: `key=value`
4. For COPY only:
   - For each source file (sorted by path):
     - Hash file content

### Extract Layers
1. Open tar file
2. For each entry:
   - Validate path security (no traversal)
   - Create directories as needed
   - Extract file with original permissions
3. Clean up on error

## Limitations & Future Work

### Out of Scope (by design)
- Networking, image registries, push/pull
- Resource limits (CPU, memory, disk)
- Multi-stage builds
- Bind mounts, detached containers
- Volume management
- Health checks, restart policies
- Signals/graceful shutdown

### Technical Constraints
- Linux-only isolation (macOS/Windows: no chroot)
- No reference counting on layers
- Single-machine operation
- No concurrent builds (would need locking)
- Layers immutable (no squashing or layer modification)

### Possible Enhancements
1. **Port mapping** - Simple iptables-based networking
2. **Layer squashing** - Merge layers before export
3. **Image export/import** - Save to tar, restore from tar
4. **Concurrent builds** - File locking in state directory
5. **Cross-platform isolation** - Mock namespaces for testing
6. **BuildKit-style caching** - Track layer dependencies
7. **Docker image compatibility** - Parse Docker image manifests

## Performance Characteristics

- **Build time**: Dominated by tar I/O and chroot syscalls
- **Cache lookup**: O(1) - single filesystem read per layer
- **Image listing**: O(n) - reads all manifests
- **Layer extraction**: O(files) - linear scan of tar entries
- **Memory**: Minimal - streams data (tar doesn't load into memory)

## Error Handling

All errors report context:
- **Docksmithfile parse errors**: Line number + instruction
- **Build failures**: Step number + error
- **Runtime errors**: Process exit code
- **File operations**: Standard Go error wrapping

## Testing Strategy (Manual)

The demo script exercises:

1. **Cold build** - All cache misses
2. **Warm build** - All cache hits  
3. **Invalidation** - Partial misses after source change
4. **Image listing** - Manifest enumeration
5. **Container execution** - Isolation verification
6. **Cleanup** - Image removal

## Security Considerations

**Not production-ready for untrusted workloads:**

- **Chroot breakout risks** - Chroot alone isn't secure; full namespaces required
- **No seccomp** - All syscalls allowed in container
- **No AppArmor/SELinux** - No mandatory access control
- **Root process** - Containers run as root inside chroot
- **No signing** - No layer signature verification

**Suitable for:**
- Educational demonstrations
- Building untrusted local code in development
- Reproducible builds of known-good sources

**Not suitable for:**
- Hosting user-submitted code
- Multi-tenant systems
- Security-sensitive production workloads
