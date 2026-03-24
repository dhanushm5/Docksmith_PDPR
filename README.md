# Docksmith - A simplified Docker-like build system built from scratch

Docksmith is an educational implementation of a Docker-like container build and runtime system in Go. It demonstrates:

1. **Build Caching** - Deterministic cache keys based on instruction content, dependencies, and environment state
2. **Content Addressing** - Layer identification using SHA-256 digests
3. **Process Isolation** - Container execution using chroot and Linux namespaces
4. **Offline Operation** - All operations work without external network dependencies

## Architecture

### Components

- **CLI**: User-facing binary with commands: `build`, `images`, `run`, `rmi`
- **Build Engine**: Parses Docksmithfile, manages layers, executes instructions with caching
- **State Directory**: `~/.docksmith/` containing:
  - `images/`: JSON manifests for each image
  - `layers/`: Content-addressed tar archives
  - `cache/`: Cache key to layer digest mappings

### Image Format

Manifests are JSON files containing:
- Image metadata (name, tag, digest, created timestamp)
- Image config (environment variables, working directory, default command)
- Layer list (digest, size, creation details for each layer)

Digest is calculated as SHA-256 of the manifest JSON with empty digest field.

### Build Language (Docksmithfile)

Six core instructions:

```
FROM <image>:<tag>        - Load base image layers
COPY <src...> <dest>      - Copy files from build context (supports globs)
RUN <command>             - Execute command in isolated container
WORKDIR <path>            - Set working directory (doesn't create layer)
ENV <key>=<value>         - Set environment variable (doesn't create layer)
CMD ["exec", "arg"]       - Set default container command (JSON array required)
```

## Build Process

1. Parse Docksmithfile and validate instructions
2. For each instruction:
   - Calculate deterministic cache key
   - Check if cached layer exists
   - If hit: reuse layer, print `[CACHE HIT]`
   - If miss: execute instruction, create layer, cache result
   - Once a miss occurs, all subsequent steps are misses (cascade)
3. Create and save image manifest with all layers

## Cache Key

For each layer instruction, the cache key is computed from:

- The full instruction text
- Current WORKDIR value
- Current ENV state (lexicographically sorted)
- For COPY: SHA-256 hash of each source file's content

This ensures reproducible, deterministic builds.

## Runtime

To run a container:

1. Create temporary directory
2. Extract all image layers in order
3. Apply chroot to root filesystem
4. Set working directory and environment
5. Execute command in isolated process
6. Clean up temporary directory

Process isolation uses:
- `chroot`: Changes root filesystem
- `CLONE_NEWPID`: Process namespace isolation
- `CLONE_NEWUTS`: Hostname namespace isolation
- `CLONE_NEWIPC`: IPC namespace isolation

## Usage

### Build an image

```bash
docksmith build -t myimage:latest /path/to/context
```

### List images

```bash
docksmith images
```

### Run a container

```bash
docksmith run myimage:latest
docksmith run -e VAR=value myimage:latest [command]
```

### Remove an image

```bash
docksmith rmi myimage:latest
```

## Sample Application

The `sample/` directory contains a complete example app that uses all six instructions:

```
Docksmithfile          - Build configuration
app/
  app.py               - Python application
  inputs/
    hello.txt          - Input data
    info.txt           - More input data
```

## Demo

Run the demo script to see:

1. Cold build (all cache misses)
2. Warm build (all cache hits)
3. Source modification (partial misses)
4. Image listing
5. Container execution
6. Environment overrides
7. Process isolation
8. Image removal

```bash
bash ./demo.sh
```

Note: Container execution requires:
- Linux OS (for chroot and namespaces)
- Root privileges
- A base image (ubuntu:20.04 or similar)

## Constraints & Design Decisions

- **No daemon**: Single CLI binary, all state on disk
- **Offline**: Base images imported once; no runtime downloads
- **Deterministic**: Tar entries sorted, timestamps zeroed for identical digests
- **Linux-only**: Uses OS primitives (chroot, namespaces)
- **No reference counting**: Deleting one image may break others sharing layers

## Implementation Notes

- All JSON written with stable field order and 2-space indentation
- Manifest digest included in final file for verification
- Cache keys use sorted environment variables
- Layers are immutable once written
- Build failures show filename and line number
- Glob patterns support `*` and `**` (filepath.Glob)

## Limitations (Out of Scope)

- Networking, image registries
- Resource limits (CPU, memory)
- Multi-stage builds
- Bind mounts, detached containers
- Volume management
- Health checks, restart policies
