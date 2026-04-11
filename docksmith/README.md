# docksmith

`docksmith` is a simplified, fully offline container build and runtime CLI inspired by Docker.

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
- No network calls are performed by the codebase.
