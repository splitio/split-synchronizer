# Docker build of `docker/Dockerfile.proxy` fails on Colima

Status: investigating. No fix applied. `docker/` was not changed.

## Summary

`docker build -f docker/Dockerfile.proxy .` fails or hangs during the builder stage (`make split-proxy`). The failures are Go module downloads from `proxy.golang.org` inside the build container. The same downloads succeed from the host and from a plain container `curl`, so the problem looks intermittent and specific to the Colima VM or to concurrent module downloads. This is a hypothesis, not a confirmed cause.

## Environment

| Item | Value |
|---|---|
| Host | macOS 25.6.0, arm64 |
| Docker CLI and daemon | Docker 29.5.2, active context `colima` |
| Colima profile | `default`, Running, aarch64, 4 CPUs, 8 GiB RAM, 100 GiB disk |
| Colima VM | macOS Virtualization.Framework, mountType `virtiofs` |
| Base image in Dockerfile | `golang:1.26.10-trixie` (pulls fine on its own) |

## Observed failures

Commands run from the repo worktree at `22b0d89` (SSYNC-2 branch).

1. Attempt 1 and 2 of the real build: failed in the builder stage with
   `read "https://proxy.golang.org/.../<module>.zip": stream error: stream ID N; INTERNAL_ERROR; received from peer`.
   - Attempt 1 failed on `github.com/splitio/go-split-commons/v10@v10.2.1`.
   - Attempt 2 failed on `github.com/google/uuid@v1.3.0`.
   - Both then ended with `make: *** [Makefile:61: split-proxy] Error 1`.
2. Attempt 1 of a later run stalled for more than 30 minutes on the line
   `go: downloading github.com/splitio/go-toolkit/v5 v5.5.1` with no further output. It was killed manually.
3. Probe image (`go mod download` on the repo's `go.mod` and `go.sum`, run twice in separate `RUN` steps):
   - Step 1: `go mod download` exited 1 after about 200 s. The `INTERNAL_ERROR` grep was not captured, and the `RUN` still succeeded because the wrapper's `echo` returned 0.
   - Step 2: `go mod download` exited 0 in about 1.4 s, because modules fetched by step 1 were already in the module cache layer.

## What works

- Host: `curl -I https://proxy.golang.org/github.com/google/uuid/@v/v1.3.0.zip` returns `200` over HTTP/2.
- Container: five sequential `curl` calls to the same URL in `golang:1.26.10-trixie` returned `200` over HTTP/2 each time.
- Host: `go mod download -json github.com/google/uuid@v1.3.0` succeeds.

## Hypotheses (unverified)

- H1: intermittent HTTP/2 stream resets (`INTERNAL_ERROR`) on the Colima VM's network path when Go starts many module downloads in parallel.
- H2: the Go download hang (failure 2) is a stalled connection with no client timeout in the `go` toolchain run, so the build never fails on its own.
- H3: Colima's `virtiofs` or VM DNS/NAT behaves differently from the host for long-lived connections. Not tested.

## Proposed next checks (not yet run)

1. Repeat `go mod download` in a container 5 times with `GODEBUG=http2debug=1` (or `GOPROXY` pointed at the same URL with `-x`), and record how often `INTERNAL_ERROR` appears.
2. Limit parallelism with `GOMAXPROCS=2` (or a `go` env) to test H1.
3. Set `GODEBUG=http2client=0` to force HTTP/1.1, which would confirm or rule out H1.
4. Compare with Docker Desktop (`desktop-linux` context) on the same Dockerfile, to separate Colima from the image and Dockerfile.
5. Check Colima VM DNS and MTU: `colima ssh -- cat /etc/resolv.conf`, then `colima ssh -- ip link`.

## Workarounds to consider (need approval, since `docker/` is protected)

- Retry the build (intermittent; a retry may succeed).
- Pre-warm the module cache in a separate stage, or use BuildKit cache mounts for `/go/pkg/mod`.
- Use a `docker build --network host` run, if the VM networking is the cause.
- Run the build on Docker Desktop, if Colima is the cause.

Any change to `docker/Dockerfile.proxy` requires approval.
