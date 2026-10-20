# LinkTadoru threat model

LinkTadoru is a command-line web crawler for link analysis. An operator runs it
locally against sites they choose; it fetches pages, follows links within a
configured scope, honors robots.txt, and stores pages and the link graph in a
local SQLite database.

## Trust boundaries

Untrusted: everything a crawled server returns — status codes, headers,
redirect targets, HTML bodies, links, and robots.txt. Assume any crawled site,
and any site it redirects or links to, may be malicious.

Trusted: the operator and everything they supply — CLI flags, `LT_*`
environment variables, the config file, seed URLs, the seed file, and include
and exclude patterns. The config file is treated as the operator's own input;
do not report that a config file can choose where credentials are sent.

## Security properties

- Credentials (Basic auth, Bearer token, API key header, custom headers,
  cookies) are only sent to origins the operator authorized. A redirect, a
  discovered link, or robots.txt must not cause them to reach another origin.
  Redirect handling strips them once the chain leaves the initial origin, and
  must not restore them on a later hop back.
- The crawl stays in scope: the seed origins plus absolute `include_patterns`
  ranges, minus `exclude_patterns`, restricted to `allowed_schemes`
  (http/https by default). Redirects and discovered links must not escape
  this scope, including through URL parsing or normalization differences.
- Responses are capped at `max_response_size` (10 MiB by default). Hostile
  responses must not cause unbounded memory, CPU, disk use, or hangs.
- Page content must not alter the SQLite database beyond the intended rows
  (no SQL injection) or write files outside the configured database and log
  paths.
- Secrets must not be written to logs or the database.

`internal/crawler` (`http_client.go`, `url_policy.go`, `crawler.go`,
`robots.go`), `internal/parser`, `internal/storage`, and `internal/config`
are where these properties are enforced.

## Out of scope

- Anything that requires control of the operator's flags, environment,
  config file, or seed file.
- Fetching a URL the operator placed in scope, including private or loopback
  addresses. There is no SSRF protection by design; it is a local tool and
  the operator picks the targets. A redirect or link that reaches such an
  address _outside_ the configured scope is in scope.
- Ignoring robots.txt when the operator set `ignore_robots_txt`.
- `--show-config` printing the operator's own secrets; it is deliberately
  unredacted.

## Severity

- High: credentials sent to an origin the operator did not authorize; code
  execution or arbitrary file write from crawled content.
- Medium: crawling outside the configured scope; hostile responses causing
  memory or CPU exhaustion, a hang, or a crash; SQL injection that corrupts
  stored data; secrets written to logs or the database.
- Low: robots.txt parsing mistakes, incorrect link-graph data, and other
  correctness bugs without a security impact.

## Offline build and reproduction

The checkout is at `/src` and the binary at `/usr/local/bin/linktadoru`. Go,
the module cache, and the build cache are in the image; `GOTOOLCHAIN=local`,
`GOPROXY=off`, and `GOSUMDB=off` prevent downloads. Tests use `httptest`
servers, so a reproducer can stand up a malicious site the same way:

```sh
cd /src && go test -race ./...
```
