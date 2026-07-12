# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is `onekit`: a from-scratch schema language (`.onk`) and toolchain for building HTTP APIs. There is **no protobuf, no buf, no protoc** anywhere in this repository — the parser, the intermediate representation, and all four generator backends are custom, built entirely against the standard library plus a small number of targeted third-party libraries (`pb33f/libopenapi` for OpenAPI documents, `BurntSushi/toml` for project config, `go.yaml.in/yaml` for YAML marshaling).

This is a rewrite-in-progress: an earlier version of onekit generated code from `.proto` files via six `protoc-gen-onekit-*` plugins. That system, and every example that depended on it, has been deleted. `examples/onk-simple-api` is the one surviving example and the reference for how the new pipeline is meant to be used.

## Pipeline

```
.onk source files
      │  internal/onklang   (lexer + recursive-descent parser → AST)
      ▼
   AST
      │  internal/onkcompile (two-pass compiler: declare names, then resolve
      │                       field/oneof-variant/RPC types across files)
      ▼
onkir.Package          (internal/onkir — the native IR; zero protobuf-go)
      │
      ├─ internal/gengo        → Go structs, validation, HTTP server, HTTP client
      ├─ internal/gents        → TS types, fetch client, Web-Fetch-API server routes
      ├─ internal/genpy        → Python dataclasses, urllib client
      └─ internal/genopenapi   → OpenAPI 3.1 document (YAML/JSON)
```

`cmd/onek` + `internal/onek` is the CLI that ties this all together: it reads `onekit.toml`, walks a project directory for `*.onk` files, compiles them, and invokes whichever generators are configured under `[generate.*]`.

## The `.onk` language

No explicit field numbers (wire-format concern that doesn't apply once protobuf is gone), no separate options-extension mechanism — attributes are `@decorator(args)` directly on the field/method/message they apply to.

```
package example.users

/// A system user.
message User {
  id: string
  name: string @len(1, 100)
  email: string @email
  bio: string? @nullable
  tags: string[]
  labels: map[string, string]
}

enum Status {
  ACTIVE @json("active")
  INACTIVE @json("inactive")
}

message Event {
  id: string
  payload: oneof(discriminator: "type", flatten: true) {
    text: TextPayload @tag("text")
    image: ImagePayload @tag("image")
  }
}

message NotFoundError @status(404) {
  resource_type: string
  resource_id: string
}

service UserService {
  base_path: "/v1"
  headers: {
    "X-API-Key": string @required @format("uuid")
  }

  createUser(CreateUserRequest) -> User @post("/users")
  getUser(GetUserRequest) -> User | NotFoundError @get("/users/{id}")
  searchUsers(SearchUsersRequest) -> SearchUsersResponse @query("/users/search")
  streamEvents(StreamEventsRequest) -> Event @get("/events") @stream  # not yet implemented by any generator
}
```

Key syntax points:

- **Types**: `string bool int32 int64 uint32 uint64 float32 float64 bytes timestamp`. `Type?` = optional/nullable, `Type[]` = repeated, `map[K, V]` = map.
- **Field validation**: `@email @uuid @len(min, max) @range(min, max) @in("a", "b") @required` — implemented as generated stdlib checks (regex/length/bounds), not a CEL/protovalidate-style runtime.
- **HTTP verbs on RPC methods**: `@get(path) @post(path) @put(path) @delete(path) @patch(path) @query(path)`. `@query` is the IETF `QUERY` method (safe like GET, but carries a body) — supported end to end (parser → all four generators).
- **`@body("field")`**: bind the HTTP body to a single sub-message field instead of the whole request.
- **`@stream`**: marks an RPC as SSE. Parsed, but **no generator implements it yet** — this is the biggest known gap.
- **Oneofs**: `field: oneof(discriminator: "...", flatten: bool) { variant: Type @tag("...") }` — a field-level construct, not a standalone block. Fully typed as discriminated unions in Go (interface + variant wrapper structs, with generated `MarshalJSON`/`UnmarshalJSON` — see "Oneof JSON in Go" below) and TypeScript (native union types). In Python, oneof fields are currently a plain `dict` pass-through, not strongly typed — a known scope cut, not a bug.
- **Error unions on RPC methods**: `-> Success | ErrorA | ErrorB` puts a method's possible errors in the schema itself. Any message ending in `Error` is treated as an error type by convention regardless of whether it's declared in a union.
- **`@status(code)` on an `*Error` message**: the HTTP status that error serializes with. Every generator's error-response handling reads this (`onkir.Message.StatusCode()`); default is 500 if absent.
- **Doc comments**: `///` (triple-slash) immediately before a declaration. A plain `//` or `/* */` comment breaks the doc block instead of extending it. Flows into `onkir.*.Doc` and from there into whatever each generator does with it (currently: nothing renders it yet except it's available — check before assuming a generator surfaces it).
- **Query params**: declared on the request message's own fields via `@query("name")`, not on the service.

**Not yet supported anywhere in the pipeline**: SSE streaming (`@stream` parses but no generator acts on it), and the old system's `flatten`/`unwrap`/`int64_encoding`/`enum_encoding`/`timestamp_format`/`bytes_encoding` JSON-mapping decorators (none of these exist in the new grammar or generators at all — don't assume they work just because the old CLAUDE.md documented protobuf-based equivalents).

## Oneof JSON in Go

This is worth understanding before touching `internal/gengo/types.go`: a Go `interface`-typed field can't be unmarshaled by `encoding/json` without help — there's no way to know which concrete variant to construct until the discriminator is read out of the raw JSON. Messages with a oneof field get generated `MarshalJSON`/`UnmarshalJSON` methods using a `type alias X` trick: a struct embeds `*alias` (a type-identical-but-method-less copy of the message) plus a shallower `json.RawMessage` field for each oneof, so the raw-JSON field wins the same-key conflict against the promoted one without infinite recursion into the real Marshal/Unmarshal. See `writeOneofMarshalJSON`/`writeOneofUnmarshalJSON` in `internal/gengo/types.go`, and `TestGeneratedServerBuildsAndServes` in `internal/gengo/gengo_test.go` for the regression test that caught this the first time (it was only caught by the `examples/onk-simple-api` end-to-end proof, not by gengo's own unit tests — the lesson being that per-package tests didn't cover a real oneof-over-HTTP round trip until that example forced it).

## Development Commands

### Testing

```bash
./scripts/run_tests.sh              # full suite with coverage analysis (85% threshold, informational)
./scripts/run_tests.sh --fast       # no coverage, faster
./scripts/run_tests.sh --verbose

go test ./...                       # equivalent, no coverage gate
go test ./internal/gengo/... -v     # single package
```

Several tests shell out to other toolchains and skip gracefully if they're missing: `internal/gents` needs `tsc` (`npm install -g typescript`) and `node` on PATH; `internal/genpy` needs `python3`. CI installs TypeScript globally before running tests — if you're missing it locally, those specific tests report `--- SKIP`, not failure.

### Building

```bash
make build                  # builds ./bin/onek
go build -o bin/onek ./cmd/onek   # equivalent, direct
go fmt ./...
```

### Using the CLI

```bash
./bin/onek check <dir>    # parse + compile only, no codegen - fast validation
./bin/onek build <dir>    # parse + compile + generate everything in <dir>/onekit.toml
```

`onekit.toml`:

```toml
module = "github.com/you/yourapp/api"

[generate.go-server]
out = "./api"

[generate.go-client]
out = "./api"          # same dir as go-server is fine - they share the package

[generate.ts-client]
out = "./web/client"

[generate.ts-server]
out = "./web/server"

[generate.python-client]
out = "./python_client"

[generate.openapi]
out = "./docs"
title = "Your API"
version = "1.0.0"
```

`internal/onek.Build` merges every compiled `onkir.File` in a project into one before handing it to a generator (see `mergeFiles` in `internal/onek/build.go`) — a project's `.onk` sources are typically split across files by concern (models vs. services) but target one generated Go package (or one TS/Python module) per output directory, and Go enforces one package per directory regardless of how many onk `package` declarations exist.

## Testing Strategy

Every generator package proves itself the same way: **generate real code from a fixture, then actually run it**, not just diff against golden files or check that generation didn't error.

- `internal/gengo`: writes generated Go to a temp module and does `go run .` against an in-process `httptest` server, driving the generated client against the generated server — create, get, a 404 with a typed error, oneof round-tripping, and a validation rejection.
- `internal/gents`: `tsc --noEmit` for a static type-check proof, plus a separate runtime proof that invokes generated route handlers directly with constructed `Request` objects under `node` (Node's native TS execution, no bundler).
- `internal/genpy`: spins up a real `http.server`-based mock and drives it with the generated client.
- `internal/genopenapi`: round-trips the generated YAML through `pb33f/libopenapi`'s own `NewDocument` + `BuildV3Model` — the strongest available proof the output is actually valid OpenAPI, not just well-formed YAML.
- `internal/onek`: an end-to-end `Build()` test that writes an `onekit.toml` + `.onk` files to a temp dir and confirms the generated Go package `go build`s.
- `examples/onk-simple-api` is the outermost proof: a real project directory, generated once and committed, with its own `go test` exercising create/get/404/oneof-login/validation-rejection through the generated client against the generated server. This is what actually caught the oneof-JSON bug — package-level tests hadn't exercised that path.

When adding a decorator or feature, prefer extending an existing runtime-proof test over adding a new golden-file comparison — the whole point of this testing style is catching things like the oneof bug that "the generator ran without error and produced plausible-looking code" wouldn't catch.

## Key Implementation Details

- **`internal/onkir`** is intentionally boring: plain Go structs (`Package`, `File`, `Message`, `Field`, `Type`, `Enum`, `Service`, `Method`, `Header`, `Decorator`), no interfaces, no protobuf-go types anywhere. Helper methods (`Method.Verb()`, `Header.Required()`, `Message.StatusCode()`, etc., in `internal/onkir/helpers.go`) interpret well-known decorators so generators don't re-parse decorator args at every call site — but the raw `Decorators []Decorator` is always still there for anything a helper doesn't cover yet.
- **`internal/onkcompile`** does name resolution in a flat namespace: every message/enum (including nested ones) across every file passed to `Compile()` is registered by simple name, and duplicates are a compile error. There's no dotted cross-package qualification — if you need one file to reference a type from another, they just have to be compiled together (which `internal/onek.Build` always does — see `discoverOnkFiles` + `parseSources`).
- **Field/method-name conventions differ slightly per language** in the generators: Go and TS use `PascalCase`/`camelCase` conversions from the onk source's `snake_case`/`camelCase`; Python keeps `snake_case` field names as-is (already idiomatic) but converts RPC method names from the onk source's `camelCase` to `snake_case` for the generated client methods.
- **Generated Go/TS/Python client and server code has zero dependency on this repo** — everything gengo/gents/genpy produce is self-contained against each language's standard library. `examples/onk-simple-api/go.mod` has no `require` block at all as a direct consequence.

## Project Structure

- **`cmd/onek/`**: CLI entrypoint (`build`/`check`/`generate`; `fmt` is a stub, not implemented).
- **`internal/onklang/`**: tokenizer, recursive-descent parser, AST for `.onk`.
- **`internal/onkcompile/`**: AST → `onkir.Package`, with cross-file type resolution and compile-time errors (unresolved types, duplicate names, invalid header/map-key types, etc.).
- **`internal/onkir/`**: the native IR every generator consumes.
- **`internal/onek/`**: `onekit.toml` parsing (`config.go`) and the `Build`/`Check` orchestration (`build.go`).
- **`internal/gengo/`**: Go generator — `types.go` (structs/enums/oneof unions + their JSON marshal/unmarshal), `validate.go` (field validation methods), `server.go` (`net/http` `ServeMux`-based routing), `client.go` (HTTP client).
- **`internal/gents/`**: TypeScript generator — `types.go`, `client.go` (fetch-based), `server.go` (Web Fetch API route descriptors).
- **`internal/genpy/`**: Python generator — `types.go` (dataclasses + enums), `client.go` (urllib-based).
- **`internal/genopenapi/`**: OpenAPI 3.1 generator built on `pb33f/libopenapi`'s high-level v3 model.
- **`examples/onk-simple-api/`**: the one example. `.onk` sources, `onekit.toml`, and the committed generated output (`api/*.gen.go`, `docs/openapi.yaml`) side by side, so it doubles as a "here's literally what onek produces" reference.
- **`scripts/run_tests.sh`**: test runner with coverage analysis and reporting.

## Acknowledgments

- **[pb33f/libopenapi](https://github.com/pb33f/libopenapi)** — the OpenAPI 3.1 high-level model `internal/genopenapi` builds documents against, and the validator (`NewDocument` + `BuildV3Model`) its tests use to prove generated specs are actually valid.
- **[BurntSushi/toml](https://github.com/BurntSushi/toml)** — `onekit.toml` project-file parsing. The one third-party dependency added specifically for this project's own tooling (not inherited from the earlier protobuf-based design).
- **[go.yaml.in/yaml](https://github.com/yaml/go-yaml)** / **[sigs.k8s.io/yaml](https://github.com/kubernetes-sigs/yaml)** — YAML marshaling and YAML→JSON conversion for OpenAPI output.
- **Go, TypeScript, and Python standard libraries** — every generated client/server is stdlib-only by design; no generated code depends on this repo or on any runtime library at all.
