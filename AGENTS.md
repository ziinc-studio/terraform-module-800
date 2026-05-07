# AGENTS.md — Working in this repository

This is a Terraform _provider_ for the 800.com REST API
(`https://api.800.com`). It is written in Go using the
**Terraform Plugin Framework** (not SDKv2). The provider name on the
Terraform Registry is `eighthundred`; resources are prefixed
`eight_hundred_` (Terraform identifiers cannot start with a digit).

The companion plan in [PHASED-EXECUTION.adoc](PHASED-EXECUTION.adoc) is
authoritative for scope. Read it before starting work.

## Repo layout

```
.
├── main.go                      # provider entry point
├── internal/provider/           # all provider code lives here
│   ├── provider.go              # provider config (token, endpoint, default_company_id)
│   ├── client/                  # HTTP client, auth, rate-limit, pagination
│   ├── resource_*.go            # one file per managed resource
│   └── data_source_*.go         # one file per data source
├── examples/                    # tfplugindocs reads these
├── docs/                        # generated; do not hand-edit
├── public.v2.swagger.yml        # vendored API spec for reference
├── PHASED-EXECUTION.adoc        # plan / scope
├── README.adoc
└── Makefile
```

## API ground truth

- Base URL: `https://api.800.com`.
- Auth: `Authorization: Bearer <token>`.
- Token format: `<userId>|<opaque>`.
- Rate limit headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`,
  `Retry-After`. **The spec text claims 60/min; live servers return
  180/min. Trust the headers, never a hardcoded number.**
- Response envelope: `{"data": ...}` for success.
- Three pagination shapes coexist (cursor v2, page snake_case, page
  camelCase) — see PHASED-EXECUTION §0 for examples. The HTTP client
  must abstract them so resource code is shape-agnostic.

## Conventions

### Naming

- Go package: `provider`.
- Resource type names in HCL: `eight_hundred_<noun>` (singular).
- Data source names: `eight_hundred_<noun>` (singular for one,
  plural for list).
- File names: `resource_<noun>.go`, `data_source_<noun>.go`.

### Schema

- Use Plugin Framework, not SDKv2.
- Every resource has an `id` computed string attribute (the API's
  numeric ID stringified).
- `company_id` is required on company-scoped resources but falls back to
  the provider-level `default_company_id` when omitted. Resolve in
  `Configure`, not in each CRUD method.
- Sensitive fields use `Sensitive: true`.

### CRUD

- **Create** must capture the server-assigned ID immediately, even on
  partial failure. Use single-shot retry, not exponential — duplicate
  resources from naive retry are the biggest foreseeable bug class.
- **Read** is the only place that calls the GET endpoint. Use it for
  drift detection. If the resource is gone (`404`), call
  `resp.State.RemoveResource(ctx)`.
- **Update** uses `PUT`/`PATCH` per the spec. Send only changed fields
  when the API supports `PATCH`; full object on `PUT`.
- **Delete** must be idempotent — treat `404` as success.
- **Import** ID format: `<company_id>:<resource_id>` for company-scoped
  resources, `<resource_id>` for global ones. Always document this in
  the resource's example.

### Errors

The API returns two error shapes:

```json
{"errors": ["..."], "message": "..."}        // validation, 401, 422
{"metadata": [], "data": {"message": "..."}} // 404
```

The client normalises both into a `client.APIError` with `StatusCode`,
`Code` (slug derived from message), `Message`, and `RawBody`. Resource
code maps to `diag.Diagnostics` — never log the token.

### Tests

- Unit tests: `go test ./...` — no network.
- Acceptance tests: gated by `TF_ACC=1` and `EIGHT_HUNDRED_API_TOKEN`.
  Hit `https://api.800.com` directly. **Do not** mock the API in
  acceptance tests — it defeats their purpose.
- Tests for `eight_hundred_number` cost real money. Keep them in a
  separate file with a `//go:build paid_acceptance` build tag and
  document the cost in the test comment.

## Local development

```sh
# Build into $GOPATH/bin and register a dev override.
make install

# Quick smoke test against your token.
export EIGHT_HUNDRED_API_TOKEN='377187|...'
cd examples/resources/eight_hundred_webhook
terraform plan
```

The `dev-overrides.tfrc` template at the repo root shows the
`dev_overrides` block to put in `~/.terraformrc`.

## What goes where

- *Adding a new managed resource?* `internal/provider/resource_<noun>.go`,
  register in `provider.go`'s `Resources()`, add an example under
  `examples/resources/eight_hundred_<noun>/`, run
  `make docs`, ensure acceptance tests cover Create/Read/Update/Delete.
- *Found spec drift?* Update `public.v2.swagger.yml` and note the
  divergence in PHASED-EXECUTION.adoc under "Spec drift". Don't silently
  patch the resource to match — record the drift first.
- *Need a new endpoint method on the client?* Put it on the typed client
  in `internal/provider/client/`, return Go types not raw maps, and
  always include the `company_id` argument explicitly even if the
  provider has a default — the client layer must be context-free.

## Hard rules

1. **Never commit secrets.** The provider's `token` field is sensitive;
   examples use `var.token` or `EIGHT_HUNDRED_API_TOKEN`.
2. **Never hardcode the rate limit.** Read it from headers per response.
3. **Never make the destroy path optimistic on `eight_hundred_number`.**
   That's real money. Always do a final read first.
4. **Never use `terraform-plugin-sdk/v2` for new code** — Plugin
   Framework only.
5. **Never log responses verbatim** — they may contain phone numbers
   and customer PII. Strip before logging.

## Out of scope (today)

- HCL convenience modules under `modules/` — Phase 6, only after the
  provider stabilises.
- A generated client from the OpenAPI spec — we hand-write the client
  for now; the spec has too many `App\Http\...` PHP-style schema names
  for a clean generator pass.
- The `POST /message` and ECID lookup endpoints — not stateful, so they
  do not become resources. They might become *ephemeral resources*
  (Terraform 1.10+) in a later phase.
