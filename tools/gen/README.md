# Code generator

The packages `uapi/` and `whm/` are generated from cPanel's **official
OpenAPI 3 documents**, the very same documents that power the cPanel & WHM
developer portal:

- cPanel UAPI — <https://api.docs.cpanel.net/specifications/cpanel.openapi>
- WHM API 1 — <https://api.docs.cpanel.net/specifications/whm.openapi>

The generated code currently targets **cPanel & WHM version 138**
(spec info version `11.137.9999.96`).

## Regenerating

Requirements: Python 3.9+ with PyYAML, Go 1.22+.

```sh
# 1. download the latest official specs
tools/gen/fetch-specs.sh

# 2. regenerate the uapi/ and whm/ packages
python3 tools/gen/generate.py \
    tools/gen/cpanel.openapi.yaml \
    tools/gen/whm.openapi.yaml \
    . \
    --cpanel-md tools/gen/cpanel.openapi.md \
    --whm-md tools/gen/whm.openapi.md

# 3. format & verify
gofmt -w .
go build ./...
go vet ./...
go test ./...
```

The `.md` catalog files are optional; they are only used to link every
generated function to its per-function documentation page.

## What the generator does

- Emits one file per UAPI module (`uapi/zz_<module>_gen.go`) and one per WHM
  category (`whm/zz_<category>_gen.go`).
- Every documented function gets:
  - a typed `...Args` struct (required parameters as plain fields, optional
    scalars as pointers, arrays as slices, plus an `Extra cpanel.Args` bag);
  - a typed `...Data` response payload where the schema allows it, and
    `json.RawMessage` for schemaless or union payloads (still fully
    accessible, plus the `Raw` member of every result);
  - a method with the documented HTTP verb and a doc comment distilled from
    the upstream description, including the upstream documentation URL.
- OpenAPI `oneOf`/`anyOf` unions that collapse to one scalar type use that
  type; heterogeneous unions become `any`, and object unions become
  `json.RawMessage`; `allOf` compositions merge their properties.
- Identifier names are segmented into words with a domain dictionary that is
  augmented automatically with the vocabulary observed in the specs.
