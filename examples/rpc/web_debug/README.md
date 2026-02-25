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
- Metadata mapping (`meta.actorId`, `meta.tenant`, `meta.requestId`) into `crud.Context`
- Scope guard enforcement in RPC flows
- Browser debug panel with endpoint list and request/response history

## Why this example has its own go.mod

This folder is an isolated module so you can run and iterate on the RPC demo without changing or resolving dependencies through the repository root module.

## Run

```bash
cd examples/rpc/web_debug
go mod tidy
go run .
```

Open `http://localhost:8092`.

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
