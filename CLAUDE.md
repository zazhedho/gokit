# gokit

Public Go library (`github.com/zazhedho/gokit`) of shared gin + GORM plumbing: responses, binding, list queries, the generic repository, middleware, env config and helpers.

## Working rules (owner decisions)
- **Never read `.env` files.** Use fake values in throwaway shells or containers. Run anything that loads config from a `git archive` copy, which has no `.env`.
- **Git and pull requests.**
  - Branch from an up-to-date `main` and open a PR; `main` is protected and nothing is pushed to it directly.
  - Use Conventional Commits with no attribution lines.
  - Stage files by path; `git add -A`, `--all` and `.` are not allowed.
  - Never use `git stash`.
- **Merge only when every CI check is green, including `zero-issues`:**
  - **SonarCloud must have 0 open issues and 0 hotspots to review**, on the PR and on `main`.
  - Fix issues for real. Use `// NOSONAR <rule> <reason>` only for a proven false positive.
- **Dependencies.**
  - Keep them on the latest versions: `go get -u ./... && go mod tidy`.
  - Write the Go version as the minor line only: `go 1.27` with no toolchain line, and `golang:1.27-alpine` for Docker. Move to the next minor when it ships.
- **Comments.**
  - Write none unless one is truly needed: a security reason, a library or protocol quirk, or an ordering or concurrency rule the code cannot show.
  - Keep each one to 1–2 lines.
  - Do not write doc comments that restate the declaration, narrative comments or section dividers. Prefer clearer names or a small function.
- **README.** It must stand alone: no history, no "upgrading from" sections, no "see another project's README". Migration notes go in the GitHub Release.
- **Process.**
  - Use TDD for behaviour changes: write the failing test first.
  - Run an independent review, with a security review when auth, input or infrastructure is touched, before merging.
  - Batch changes so each repo gets one release, not a string of small ones.

## Library rules
- **One import path.** The root package is a thin facade: aliases, constructors and one-line wrappers. All logic lives in `internal/`. Never split into subpackages.
- **Root files.** Exactly one facade file named after the library, plus `config.go` when the library has config. Root tests are only the public API golden test and one facade test.
- **Public API is pinned** by `TestPublicAPI` and `testdata/public_api.golden`.
  - If the golden changes, the API changed: regenerate it with `go test -run TestPublicAPI -update .` only on purpose.
  - Before v1, a breaking change bumps the minor version. Settle all breaking changes in a single release.
- **Release.** Merge to `main`, tag `vX.Y.Z`, then run `gh release create` with migration notes. Internal-only changes go to `main` without a new tag.

## gokit specifics
- **`Repository[T]` and `CRUDRepository[T]` are defined types, never generic aliases.** GoLand cannot resolve the promoted fields of an embedded generic alias.
- **Repository behaviour.**
  - `Store` and `Update` take `*T`, and `Update` uses scoped `Updates`, never `Save`.
  - `Update`, `Delete` and `SoftDelete` return `ErrNotFound` when no row matched.
  - Ordering always ends with the key as a tie-breaker.
  - Search escapes LIKE wildcards. The SQL targets Postgres.
- **List limits.** `QueryOptions.MaxLimit` defaults to 100, and `limit=-1` is honoured only with `AllowAll`. Responses report the effective limit through `QueryOptions.PageSize`.
- **Errors.** The envelope's `error` field is always a list of `{field, code, message}`, and `log_id` is a UUID.
- **CORS** is open by default (`*`, no credentials). `*` with credentials panics.
- **Rate limits.** Limiters key on `ClientIP()` by default, IPv6 is masked to /64, and a custom `KeyFunc` result is hashed. `Limit` or `Window` ≤ 0 panics.
- **Env.** `GetEnv` treats blank as unset. `EnvErrors()` reports unparsable values by name, never by value.
- **Tests.** Postgres tests run when `GOKIT_TEST_DATABASE_URL` is set; CI runs them.
- **Gate:** `go build ./... && go vet ./... && go test -race -count=1 ./... && make lint` (0 issues), and `gofmt -l .` must be empty.
