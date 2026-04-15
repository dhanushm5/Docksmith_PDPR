# docksmith-linux

`docksmith-linux` is the Linux-focused Docksmith variant, a simplified offline container build and runtime CLI inspired by Docker.

## Features

- Supports `FROM`, `COPY`, `RUN`, `WORKDIR`, `ENV`, `CMD`
- Immutable tar layers stored at `~/.docksmith/layers/`
- Image manifests stored at `~/.docksmith/images/`
- Deterministic layer tar output (sorted entries, zero timestamps)
- Build cache with explicit `[CACHE HIT]` / `[CACHE MISS]`
- Linux runtime isolation using `chroot` and namespaces
- Offline-only local image store (`FROM` resolves local images only)

## Build

```bash
make build
```

## Cleanup

Remove the compiled binary:

```bash
make clean
```

Remove the binary and local Docksmith data store:

```bash
make clean-all
```

If you want to remove just the local image store, delete `.docksmith/`.

## Linux Native (No Docker)

Run setup once to install the local `alpine:3.18` base image directly (download + unpack, no Docker required):

```bash
./docksmith setup
```

Then build and run images natively:

```bash
./docksmith build -t sample:latest sample-app
sudo ./docksmith run sample:latest
```

You can also use the helper wrapper:

```bash
./run-linux.sh setup
./run-linux.sh build -t sample:latest sample-app
sudo ./run-linux.sh run sample:latest
```

## Usage

Build image:

```bash
./docksmith build -t sample:latest .
```

Build with cache disabled:

```bash
./docksmith build -t sample:latest --no-cache .
```

Run image command:

```bash
sudo ./docksmith run sample:latest
```

Override command:

```bash
sudo ./docksmith run sample:latest /bin/sh -c "echo override"
```

List images:

```bash
./docksmith images
```

Remove image:

```bash
./docksmith rmi sample:latest
```

## ENV override demo

The sample app uses `APP_MODE` from `ENV` in `Docksmithfile`. Override it at runtime with host environment:

```bash
APP_MODE=override sudo ./docksmith run sample:latest
```

## Notes

- `RUN` during build and `docksmith run` both use the same Linux isolation path.
- Isolation requires Linux with privileges that allow namespace + `chroot` operations.
- If strict isolation is blocked by host policy/capabilities, Docksmith automatically falls back to compatibility mode so local builds/runs still work.
- When invoked with `sudo`, Docksmith reuses the invoking user's `~/.docksmith` store by default.
- Build and run are local-only; `docksmith setup` performs a one-time Alpine rootfs download.
