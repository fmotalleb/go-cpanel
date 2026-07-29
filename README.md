# go-cpanel

A complete Go client for **cPanel & WHM API function**, generated
directly from [cPanel's own API documentation](https://api.docs.cpanel.net)
(the official OpenAPI 3 documents for cPanel & WHM version 138).

| API | Coverage | Package |
| --- | --- | --- |
| **cPanel UAPI** | **642 / 642** functions across 97 modules | `uapi` |
| **WHM API 1** | **625 / 625** functions across 49 categories | `whm` |
| cPanel API 2 (deprecated) | generic executor (direct + via WHM proxy) | `cpanel` |

**1 267 typed functions** — every single function from the official spec —
with typed arguments, typed responses, doc comments and links back to the
upstream documentation. Pure standard library, no runtime dependencies.

## Install

```sh
go get github.com/fmotalleb/go-cpanel
```

```go
import (
    cpanel "github.com/fmotalleb/go-cpanel"
    "github.com/fmotalleb/go-cpanel/uapi"
    "github.com/fmotalleb/go-cpanel/whm"
)
```

## Quick start

### cPanel UAPI (port 2083)

```go
package main

import (
    "context"
    "log"

    cpanel "github.com/fmotalleb/go-cpanel"
    "github.com/fmotalleb/go-cpanel/uapi"
)

func main() {
    c, err := cpanel.NewClient("https://cpanel.example.com:2083",
        cpanel.CPanelTokenAuth("username", "CPANEL-API-TOKEN"),
    )
    if err != nil {
        log.Fatal(err)
    }
    uc := uapi.New(c)
    ctx := context.Background()

    // Create an email account.
    res, err := uc.Email().AddPop(ctx, &uapi.EmailAddPopArgs{
        Email:    "bob",
        Domain:   cpanel.String("example.com"), // optional fields are pointers
        Password: "s3cur3-p@ssw0rd",
        Quota:    cpanel.Int(int64(250)),
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("created: %s", res.Data)

    // List email accounts.
    pops, err := uc.Email().ListPops(ctx, nil)
    if err != nil {
        log.Fatal(err)
    }
    for _, acct := range pops.Data {
        log.Printf("%s", acct.Email)
    }
}
```

### WHM API 1 (port 2087)

```go
    wh, _ := cpanel.NewClient("https://whm.example.com:2087",
        cpanel.WHMTokenAuth("root", "WHM-API-TOKEN"),
    )
    server := whm.New(wh)
    ctx := context.Background()

    // Create a hosting account.
    res, err := server.CreateAcct(ctx, &whm.CreateAcctArgs{
        Username: "bob",
        Domain:   cpanel.String("bob.example.com"),
        Password: cpanel.String("s3cur3-p@ssw0rd"),
        Plan:     cpanel.String("basic"),
    })
    ...

    // List accounts.
    accts, err := server.ListAccts(ctx, nil)
    for _, a := range accts.Data.Acct {
        log.Printf("%s — %s (%s)", a.User, a.Domain, a.Plan)
    }
```

### cPanel API 2 (deprecated)

```go
    // On a cPanel server (port 2083), or through WHM on behalf of a user:
    res, err := client.WHMAPI2(ctx, "bob", "Email", "listpops", nil)
    var accounts []struct{ Email string `json:"email"` }
    _ = res.DecodeData(&accounts)
```

> cPanel API 2 is deprecated; prefer the UAPI equivalent whenever one exists.
> The generic executor covers all modules (`Email`, `AddonDomain`, `MysqlFE`,
> `Fileman`, ...).

## Authentication

One `Authenticator` per credential scheme — pass whichever you use to
`cpanel.NewClient`:

| Constructor | Scheme | Typical use |
| --- | --- | --- |
| `cpanel.BasicAuth(user, password)` | HTTP Basic | username + password |
| `cpanel.CPanelTokenAuth(user, token)` | `Authorization: cpanel …` | cPanel API tokens (recommended for UAPI) |
| `cpanel.WHMTokenAuth(user, token)` | `Authorization: whm …` | WHM API tokens (recommended for WHM) |
| `cpanel.AccessHashAuth(user, hash)` | `Authorization: whm …` | legacy WHM access hash (deprecated) |

Session URLs (`…/cpsess########`) are also supported: they need no
authenticator because the session path itself authenticates the request:

```go
c, _ := cpanel.NewClient("https://host:2083/cpsess1234567890", nil)
```

Custom transport, TLS, timeouts and headers:

```go
c, _ := cpanel.NewClient("https://host:2087",
    cpanel.WHMTokenAuth("root", "TOKEN"),
    cpanel.WithTimeout(30*time.Second),
    cpanel.WithInsecureSkipVerify(true),          // self-signed certs
    cpanel.WithHTTPClient(myHTTPClient),
    cpanel.WithHeader("X-Env", "staging"),
)
```

## Arguments

- Functions with documented parameters take a typed `…Args` struct.
  **Required** parameters are plain fields; **optional** scalars are
  pointers so that `nil` means "do not send" — helpers `cpanel.String`,
  `cpanel.Int`, `cpanel.Float`, `cpanel.Bool` make them easy to build.
- Every `…Args` struct ends with an `Extra cpanel.Args` field for anything
  not modeled, notably the UAPI/WHM **meta arguments** (filter, sort,
  paginate, columns, ...):

```go
accts, _ := server.ListAccts(ctx, &whm.ListAcctsArgs{
    Search: cpanel.String("example.com"),
    Searchtype: cpanel.String("domain"),
    Extra: cpanel.Args{}.
        Set("api.paginate", "1").
        Set("api.paginate.size", "50").
        Set("api.sort.column", "user"),
})
```

- Functions with no documented parameters take a variadic `extra ...cpanel.Args`
  instead: `uc.ServerInformation().GetServerConfig(ctx)` or with extras
  `uc.Email().ListPops(ctx, cpanel.Args{}.Set("api.paginate.size","100"))`.
- Parameters whose documented type is a union (e.g. `0` or `"unlimited"`)
  are `any`; assign plain Go values (`250`, `"unlimited"`).
- Functions documented as POST automatically send their arguments URL-encoded
  in the request body; everything else uses the query string.

## Results & errors

Every call returns a typed envelope result plus an idiomatic `error`:

```go
res, err := uc.Email().GetEmailQuotaAndType(ctx, args)
if err != nil {
    var apiErr *cpanel.Error
    if errors.As(err, &apiErr) {
        log.Print(apiErr.Op, apiErr.Reason, apiErr.Errors)
    }
}
// res is still usable on API-level failures (messages, warnings, status):
_ = res.OK()                // envelope success flag
_ = res.ErrorStrings()      // collapsed envelope errors
_ = res.MessageStrings()
_ = res.WarningStrings()
```

- `cpanel.UAPIResult[T]` — Data `T`, Status, Errors, Messages, Warnings,
  Metadata, and both UAPI wire formats (`/execute/...` and the legacy
  `/json-api/cpanel` wrapper) are normalized automatically.
- `cpanel.WHMResult[T]` — Data `T` plus typed `Metadata`
  (command/reason/result/version).
- Functions with schemaless or union payloads return `json.RawMessage` as
  their data; the raw bytes are also always on `result.Raw`, and
  `result.DecodeData(&v)` re-decodes them into your own shape.

## Package layout

```
github.com/fmotalleb/go-cpanel   core client, auth, envelopes, API 2
github.com/fmotalleb/go-cpanel/uapi   642 typed UAPI functions (97 modules)
github.com/fmotalleb/go-cpanel/whm    625 typed WHM API 1 functions (49 categories)
github.com/fmotalleb/go-cpanel/examples/{uapi,whm,api2}   runnable demos
github.com/fmotalleb/go-cpanel/tools/gen   spec fetcher + code generator
```

All generated files (`zz_*_gen.go`, and the `uapi`/`whm` `client.go`) are
machine-written from the official specs by `tools/gen/generate.py` — see
[tools/gen/README.md](tools/gen/README.md) for regeneration instructions.

## Documentation

- cPanel & WHM developer portal: <https://api.docs.cpanel.net>
- cPanel UAPI reference: <https://api.docs.cpanel.net/specifications/cpanel.openapi>
- WHM API reference: <https://api.docs.cpanel.net/specifications/whm.openapi>
- Guide to cPanel API tokens: <https://api.docs.cpanel.net/cpanel/tokens>
- Guide to WHM API tokens: <https://api.docs.cpanel.net/whm/tokens>

The generated code targets **cPanel & WHM v138**; functions annotated with
`Available since …` in their doc comments do not exist on older servers.
This project is not affiliated with or endorsed by WebPros/cPanel, LLC.

## License

[MIT](LICENSE)
