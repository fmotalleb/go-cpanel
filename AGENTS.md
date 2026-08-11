# AGENTS.md

Go client library for cPanel & WHM. Zero runtime dependencies — stdlib only
(enforced by `depguard` in `.golangci.yml`). Targets cPanel/WHM v138.

## Layout

- Root package `cpanel` — hand-written core: `NewClient`, auth constructors
  (`BasicAuth`, `CPanelTokenAuth`, `WHMTokenAuth`, `AccessHashAuth`),
  `Args`/`EncodeArgs`, result envelopes (`UAPIResult[T]`, `WHMResult[T]`),
  `Error`. This is the only package you hand-edit for API behavior.
- `uapi/`, `whm/` — **fully generated**. Every `zz_*_gen.go` **and**
  `client.go` carries a "DO NOT EDIT" header. Do not hand-edit these; change
  `tools/gen/generate.py` (or the OpenAPI spec) and regenerate.
- `tools/gen/` — spec fetcher + Python generator. `generate.py` is ~1000 lines;
  word-segmentation dictionaries and acronym `RENDERS` live at its top.
- `examples/{uapi,whm,api2}` — runnable demos (also excluded from lint).

## Generated code — regenerate, don't edit

The OpenAPI specs (`tools/gen/*.openapi.yaml|md`) are **gitignored**; a fresh
clone must download them first. `generate.py` needs `gofmt` on `$PATH` and
formats every file it writes (it warns and skips if gofmt is missing):

```sh
tools/gen/fetch-specs.sh
python3 tools/gen/generate.py \
    tools/gen/cpanel.openapi.yaml \
    tools/gen/whm.openapi.yaml \
    . \
    --cpanel-md tools/gen/cpanel.openapi.md \
    --whm-md tools/gen/whm.openapi.md
```

See `tools/gen/README.md`. Regenerating against a newer spec bumps function
counts/dates, so re-check the README numbers if you do.

## Weekly auto-update

`.github/workflows/update.yml` runs every Monday (and on `workflow_dispatch`):
fetches the specs, regenerates, verifies, and if anything changed commits to
`main`, pushes a Go-valid tag, and publishes a GitHub release. cPanel's 4-part
spec `info.version` (e.g. `11.137.9999.96`) is not a valid Go module tag, so
the workflow rewrites trailing parts as a semver prerelease → `v11.137.9999-96`.
It never commits when output is unchanged. This is the only place that pushes
generated code to `main` — leave it to the bot.

## Verify (matches CI)

```sh
gofmt -l .            # CI fails if non-empty
go build ./...        # go vet ./..., go test -race -count=1 ./... likewise
go test ./...         # runs locally; see single-package form below
```

- CI runs on Go 1.22 – 1.26 (`go.mod` says `go 1.26`). Keep code compatible
  with Go 1.22 (no newer stdlib APIs in non-test code).
- Run one package: `go test ./uapi/` (all tests use `httptest` mock servers —
  no live cPanel/WHM server needed).

## Lint gotcha

`.golangci.yml` and `golangci.log` are **stale**, copied from an unrelated
"scheduler" project: `gofumpt module-path`/`goimports local-prefixes` point at
`scheduler`, exclusions reference a non-existent `format.go`, depguard allows a
non-existent `api2` subpackage, and `golangci-lint run` currently reports ~10
real issues (depguard on test imports, `forcetypeassert`, `gocyclo`). Lint is
**not** part of CI. Don't run `golangci-lint run` and treat the result as
authoritative, and don't "fix" test imports to satisfy its depguard rule.

## Generated API conventions (match these in the generator)

- Required params are plain struct fields; optional scalars are pointers
  (`cpanel.String/Int/Float/Bool` helpers); optional arrays are slices.
- Every `...Args` struct ends with `Extra cpanel.Args` for unmapped/meta args
  (`api.paginate.*`, sort, filter, ...). Functions with no documented params
  take variadic `extra ...cpanel.Args`.
- Union-typed params become `any`; schemaless/union payloads become
  `json.RawMessage` (raw bytes always available on `result.Raw`, plus
  `DecodeData`).
- HTTP verb matters: GET/HEAD/DELETE args go in the query string, everything
  else as a `application/x-www-form-urlencoded` body (`client.go` `doRaw`).
- Envelopes are normalized automatically; `LooseString` tolerates structured
  objects inside the errors/messages/warnings string lists — don't simplify it
  back to plain `string`.
