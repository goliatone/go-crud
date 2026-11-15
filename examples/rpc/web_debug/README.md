# go-crud RPC Web Debug Example

This example adapts the `go-command` RPC web debug console to `go-crud/rpc`.

It registers a `User` CRUD controller into a `go-command/rpc.Server`, mounts transport routes using `go-router/rpcfiber`, and provides a browser UI to inspect full request/response payloads.

## What it demonstrates

- `crudrpc.RegisterResourceEndpoints` against `go-command/rpc.Server`
- Generated methods:
  - `crud.user.create`
  - `crud.user.create_batch`
  - `crud.user.show`
  - `crud.user.index`
  - `crud.user.update`
  - `crud.user.update_batch`
  - `crud.user.delete`
  - `crud.user.delete_batch`
- Metadata mapping (`meta.actorId`, `meta.tenant`, `meta.requestId`, `meta.correlationId`) into `crud.Context`
- Scope guard enforcement in RPC flows
- Browser debug panel with endpoint list and request/response history

## Why this example has its own go.mod

This folder is an isolated module so you can run and iterate on the RPC demo without changing or resolving dependencies through the repository root module.

## Run

```bash
cd examples/rpc/web_debug
export GOWORK=off GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org
export GOPRIVATE= GONOPROXY= GONOSUMDB=
# macOS: use Apple's compiler for SQLite/CGO.
export CC=/usr/bin/clang CXX=/usr/bin/clang++
go mod verify
go run .
```

Open `http://localhost:8092`. Set `RPC_WEB_DEBUG_ADDR` to override the listener (for example `127.0.0.1:8092`). The serving log is emitted after managed readiness. SIGINT/SIGTERM stop admission and allow up to five seconds for admitted requests to drain before the owned database closes; startup/shutdown failures exit nonzero.

This module selects native Fiber v3 with published CRUD v0.27.3, RPC v0.27.2 and router v0.65.3, without local replacements. `rpcfiber.Mount` forwards the same RPC transport contract across both Fiber majors. The original RPC v0.21.1 pin was not published. Qualification uses Go 1.27.3 with checksum verification enabled.

`meta.actorId` and `meta.tenant` are caller-supplied demo metadata, not production authentication. The scope guard requires an actor and filters reads by a supplied tenant. Business failures retain HTTP 200 with an `error` envelope; malformed transport requests use HTTP errors. The state/debug endpoints intentionally expose demo data.

## Validation and rollback

Run `go test -race -count=1 ./...`, `go build -o /tmp/crud-rpc-debug .`, and `go vet ./...` with the environment above. Tests cover all eight CRUD methods, metadata, tenant scoping, static assets, native/socket transports, managed cancellation, HTTP/RPC drain and deadline behavior. Browser presentation remains a separate qualification step.

The central `go-router/.ctx/specs/go-fiber-v3-adoption/support/evidence/t02-m032-2026-11-15` evidence retains a complete tested managed Fiber v2 rollback (`rollback-source.tar.gz`), including source, tests, go.mod and go.sum. Restore the entire module from that archive into an isolated checkout and rerun the same checks before switching; changing only the Fiber dependency is insufficient.

## RPC request shape

```json
{
  "method": "crud.user.create",
  "params": {
    "data": {
      "record": {
        "name": "Ada",
        "email": "ada@example.com",
        "tenant_id": "tenant-alpha"
      }
    },
    "meta": {
      "actorId": "demo-admin",
      "tenant": "tenant-alpha",
      "requestId": "req-1"
    }
  }
}
```
