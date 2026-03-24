# Docksmith - Quick Start Guide

## Building from Source

```bash
cd /Users/dhanushm/Developer/GitHub/Docksmith_PDPR
go build -o docksmith main.go
```

This creates a binary `docksmith` (~3.7 MB).

## Running the Demo

The demo script shows all features in sequence:

```bash
bash demo.sh
```

**What the demo does:**
1. Cleans previous state
2. Builds the sample image (cold build - all cache misses)
3. Lists images
4. Rebuilds without changes (warm build - all cache hits)
5. Edits source file and rebuilds (partial cache misses)
6. Attempts container execution (requires root on Linux)
7. Tests environment variable overrides
8. Demonstrates image removal

**Note:** The demo requires a base image (ubuntu:20.04). See "Setting Up Base Images" below.

## Setting Up Base Images

Docksmith requires base images to be pre-imported. Since this is an offline system, you must import base images during setup, not during build.

### Option 1: Using Docker (if available)

Extract a Docker base image to Docksmith format:

```bash
# Export Docker image as tar
docker export $(docker create ubuntu:20.04 /bin/true) > /tmp/ubuntu.tar

# Create a minimal manifest for it
# This would require a helper script - implementation would depend on your needs
```

### Option 2: Create Minimal Base Image

For testing without Docker:

```bash
mkdir -p ~/.docksmith/{images,layers}

# Create a minimal ubuntu:20.04 image with basic tools
# This requires debootstrap or similar
# For demo purposes, the system will fail gracefully if base image is missing
```

## Basic Workflow

### 1. Create a Docksmithfile

```
FROM ubuntu:20.04
COPY app.py /app/
RUN chmod +x /app/app.py
WORKDIR /app
ENV GREETING="Hello"
CMD ["python3", "app.py"]
```

### 2. Build an Image

```bash
./docksmith build -t myimage:latest /path/to/app-dir
```

Output:
```
Building myimage:latest from context /path/to/app-dir
[CACHE MISS] FROM ubuntu:20.04
[CACHE MISS] COPY app.py /app/
[CACHE MISS] RUN chmod +x /app/app.py

Build completed successfully!
Image: myimage:latest
Digest: a1b2c3d4...
```

**Breakdown:**
- First line shows image name and tag
- Each instruction shows cache hit/miss status
- Final output includes digest for verification

### 3. List Images

```bash
./docksmith images
```

Output:
```
REPOSITORY    TAG       IMAGE ID      CREATED
myimage       latest    a1b2c3d4      2024-03-24T11:50:00Z
ubuntu        20.04     e5f6g7h8      2024-03-24T11:45:00Z
```

### 4. Run a Container

```bash
./docksmith run myimage:latest
./docksmith run myimage:latest python3 app.py
./docksmith run -e GREETING="Hi" myimage:latest
```

**Requirements:**
- Linux OS (macOS/Windows: container isolation not available)
- Root privileges: `sudo ./docksmith run myimage:latest`

### 5. Remove an Image

```bash
./docksmith rmi myimage:latest
```

## Understanding Build Cache

### Cache Hits vs Misses

**Cache HIT**: Layer reused without re-execution
```
[CACHE HIT] COPY file.txt /app/
```

**Cache MISS**: Layer executed and stored
```
[CACHE MISS] RUN pip install -r requirements.txt
```

### When Do Cache Misses Occur?

Misses happen when:

1. **Instruction changes**: Modify the command
2. **Source files change**: COPY files modified (hash mismatch)
3. **Environment changes**: ENV variable changed  
4. **Working directory changed**: WORKDIR changed
5. **Cascade after miss**: Once a miss occurs, all subsequent layers miss

### Example: Cache Invalidation

```bash
# First build: all misses
./docksmith build -t app:1.0 .
# Output: [CACHE MISS] for every instruction

# No changes: all hits
./docksmith build -t app:1.0 .
# Output: [CACHE HIT] for every instruction

# File changed: cascade from COPY onward
echo "new content" > file.txt
./docksmith build -t app:1.0 .
# Output: 
# [CACHE HIT] FROM ...
# [CACHE MISS] COPY file.txt ...  ← miss because file changed
# [CACHE MISS] RUN ...            ← miss cascades
# [CACHE MISS] RUN ...            ← miss cascades
```

## State Directory Structure

Docksmith stores everything in `~/.docksmith/`:

```
~/.docksmith/
├── images/                     # Image manifests (JSON)
│   ├── ubuntu_20.04.json
│   └── myimage_latest.json
├── layers/                     # Immutable layer tars (named by digest)
│   ├── a1b2c3d4e5f6g7h8...    # SHA-256 digest
│   ├── i9j0k1l2m3n4o5p6...
│   └── ...
└── cache/                      # Cache keys → layer digests
    ├── abc123def456...        # Key from hashing instruction
    ├── ghi789jkl012...
    └── ...
```

**Key points:**
- Each image is a JSON manifest
- Each layer is a tar archive (immutable once written)
- Cache keys are SHA-256 hashes
- Safe to delete cache/ directory (will rebuild)
- Deleting layers/ may break images

## Docksmithfile Instruction Reference

### FROM (required, must be first)
```
FROM imagename:tag
```
- Loads base image
- Fails if image doesn't exist locally
- Tag defaults to "latest" if omitted

### COPY
```
COPY source.txt /app/
COPY *.py /app/
COPY dir/ /app/
```
- Copies files from build context
- Supports glob patterns
- Creates destination directories
- Fails if no files match pattern

### RUN
```
RUN apt-get update && apt-get install -y python3
RUN chmod +x /app/script.sh
```
- Executes shell command
- Working directory and environment applied
- Must succeed (non-zero exit fails build)

### WORKDIR
```
WORKDIR /app
```
- Sets working directory for RUN and CMD
- Creates path if missing
- Does NOT create layer
- Persists to subsequent layers

### ENV
```
ENV APP_ENV=production
ENV DEBUG=false
```
- Sets environment variable
- Format: KEY=VALUE
- Injected into containers
- Available to subsequent RUN commands
- Does NOT create layer

### CMD
```
CMD ["python3", "app.py"]
CMD ["bash"]
```
- Sets default container command
- Must be JSON array format
- Used when docksmith run called without command
- Does NOT create layer

## Troubleshooting

### "image not found: ubuntu:20.04"
**Cause:** Base image not imported to `~/.docksmith/images/`  
**Fix:** Import base image or use existing image name

### "no files matched pattern"
**Cause:** COPY pattern doesn't match any files  
**Fix:** Check glob pattern in Docksmithfile and verify files exist in context

### "RUN failed: command exited with code"
**Cause:** Command in RUN instruction failed  
**Fix:** Check command syntax and ensure required packages installed in base image

### "container runtime requires root privileges"
**Cause:** Trying to run container without sudo  
**Fix:** Use `sudo ./docksmith run image:tag` on Linux

### "chroot not supported" (macOS)
**Cause:** OS doesn't support chroot/namespaces  
**Fix:** Run on Linux, or use this for build-only workflows (no container execution)

## Advanced Usage

### Building with No Cache
```bash
# Not implemented yet - requires -no-cache flag
./docksmith build -t image:tag context
```

### Building Multiple Tags
```bash
./docksmith build -t app:latest context
./docksmith build -t app:v1.0 context
# Both point to same layers, different manifests
```

### Viewing Layer Information
```bash
# Inspect a manifest (JSON format)
cat ~/.docksmith/images/myimage_latest.json | python3 -m json.tool

# Check layer sizes
ls -lh ~/.docksmith/layers/
```

### Copying Images Across Machines
```bash
# On source machine
tar -czf image.tar.gz ~/.docksmith/

# On destination machine
tar -xzf image.tar.gz -C ~/
```

## Implementation Highlights

### Deterministic Builds
- All tar entries sorted alphabetically
- Timestamps zeroed (1970-01-01)
- SHA-256 digests always reproducible
- Same source → identical digest

### Efficient Caching
- Cache key includes: instruction text + environment + file hashes
- Layer reuse across images sharing base systems
- Cascade invalidation prevents inconsistent states

### Offline Operation
- Zero network access required
- Base images pre-imported and cached
- All operations purely local I/O

### OS Isolation (Linux only)
- chroot: isolates filesystem root
- CLONE_NEWPID: separate process ID namespace
- CLONE_NEWUTS: separate hostname
- CLONE_NEWIPC: separate IPC space

## Next Steps

1. **Create a Docksmithfile** - Write your own in sample/
2. **Build an image** - Test with `build` command
3. **Run a container** - Use on Linux with `sudo run`
4. **Study the code** - Read ARCHITECTURE.md for deep dive
5. **Extend features** - Reference the TODOs in source

## Resources

- **README.md** - System overview and constraints
- **ARCHITECTURE.md** - Technical design and implementation details
- **sample/Docksmithfile** - Example build configuration
- **demo.sh** - Complete working example
- **Source code** - Heavily commented for learning

## Getting Help

All commands output usage and errors to stderr. Check output for line numbers when debugging Docksmithfile issues.

```bash
./docksmith              # Shows usage
./docksmith build        # Shows build-specific help
./docksmith images       # Lists all images
```
