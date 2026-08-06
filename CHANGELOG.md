# Changelog

All notable changes to LinkTadoru are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.10.0] - 2026-08-06

### Added
- `--seed-file` accepts seed URLs from a file, or from standard input with `-`.
  Blank lines and `#` comments are ignored. Either explicit CLI form replaces
  configured `seed_urls`; file and positional input are intentionally mutually
  exclusive.
- `max_depth` / `--max-depth` bounds a crawl by link distance from the supplied
  seeds. Seeds are depth 0, depth N is included, and 0 remains unlimited. Links
  beyond the bound stay in the link graph without being fetched.
- The final crawl summary distinguishes completed, unavailable, unreachable,
  skipped, unfinished, and graph-only discovered pages.

### Changed
- `max_depth=1` now schedules ready depth-1 pages without waiting for every
  in-flight seed, so one slow landing page occupies only its own worker. Deeper
  bounded crawls retain strict depth layers to keep shortest-path bounds correct.
- HTTP 408, 429, 500, 502, 503, and 504 responses are retried up to the existing
  three-attempt limit. `Retry-After` is honored with a 60-second cap, other
  retries use deterministic backoff, and the observed status, headers, timing,
  and size remain available for inspection. Exhausted transient responses stay
  errors rather than being reported as successfully crawled pages. Retryable
  work is drained within the run and recognized when resuming the same database.

## [0.9.2] - 2026-08-05

### Security
- Redirect targets are now re-checked against the host policy on every hop.
  Only the hop count was bounded before, so a redirect could steer the crawler
  onto a host it was never allowed to reach — including link-local and loopback
  addresses — even with `follow_external_hosts: false`.
- Credentials are removed once a redirect crosses to a different origin:
  `Authorization`, `Proxy-Authorization`, `Cookie`, the configured API-key
  header, and any custom headers. Go's built-in redirect policy does not cover
  the API-key and custom headers, so they were forwarded to whatever host a
  redirect named.
- Affected versions: v0.5.0 through v0.9.1. Upgrade to v0.9.2.
  See [GHSA-2692-7f24-52v6](https://github.com/masahif/linktadoru/security/advisories/GHSA-2692-7f24-52v6).

## [0.9.1] - 2026-07-03

### Changed
- Toolchain: Go 1.25 and golangci-lint v2 in CI (new lint findings fixed:
  unchecked `Close()` return values, tagged-switch cleanups).
- Dependencies: golang.org/x/net 0.33.0 → 0.56.0 (includes upstream security
  fixes), golang.org/x/time 0.12.0 → 0.15.0, cobra 1.9.1 → 1.10.2,
  viper 1.20.1 → 1.21.0; GitHub Actions: checkout v7, codecov-action v7,
  download-artifact v8, golangci-lint-action v9.
- Documentation overhaul (no code changes):
  - Deduplicated content into single sources of truth: the options table in
    `docs/configuration.md` (config/auth/headers), `.github/workflows/README.md`
    (CI/release triggers), `docs/versioning-and-releases.md` (release process),
    and `docs/development.md` (build/test setup, project structure).
  - Fixed factual drift: numeric `--delay`/`request_delay` values instead of
    invalid duration strings, `LT_` environment variables documented as
    flag-bound only (`log_*`/`allowed_schemes` are config-file only), release
    artifact names without version suffix and including Linux ARM64, removed
    nonexistent `.sha256`/`develop`-branch/`.actrc` claims, forbidden header
    list corrected, and the retry mechanism described as it is implemented
    (post-crawl requeue of `network_error` pages, 3 attempts, no backoff).
  - Replaced the schema SQL dump in the technical specification with a pointer
    to `internal/storage/schema.go` plus the design rationale (unified pages
    table, JSON headers with generated columns, views).
  - Documented crawl behavior in `docs/basic-usage(.ja).md`: page status
    lifecycle, retries, robots.txt `Crawl-delay` handling, and safe
    interrupt/resume.
  - Completed the `docs/README(.ja).md` index and added a Logging section to
    `docs/configuration.md`. All changes mirrored across English/Japanese pairs.

## [0.9.0] - 2026-07-03

### Fixed
- Retry mechanism never fired — two independent bugs: the retryable-error
  filter matched error types that were never written, and the retry phase
  itself was unreachable (the last exiting worker cancelled the crawl
  context). Transient `network_error` failures now get one retry pass per
  run, bounded by a total of 3 attempts across runs; deterministic failures
  are not retried.
- Invalid `include_patterns` / `exclude_patterns` regexes were silently
  ignored. They are now validated at startup and abort the run with a clear
  error message.
- URL fragments (`#section`) are stripped from discovered links, so anchors on
  the same page no longer produce duplicate rows and duplicate crawls.
- Timestamps are stored in a fixed-width UTC format so queue ordering and
  stale-processing cleanup are independent of local timezone and DST changes.
- Documentation drift: `ignore_robots` → `ignore_robots_txt` (config key),
  `--ignore-robots` → `--ignore-robots-txt` (CLI flag), and the pages status
  lifecycle in the technical specification now matches the implementation
  (`discovered` → `pending` → `processing` → `completed`/`skipped`/`error`).

### Added
- `max_response_size` config / `--max-response-size` flag: response bodies are
  capped (default 10 MiB) so oversized responses cannot exhaust memory;
  oversized pages are recorded as `response_too_large` errors.
- robots.txt `Crawl-delay` is now honored when it is slower than the
  configured request delay.
- Graceful shutdown on SIGINT/SIGTERM: in-flight state is persisted and the
  database is closed cleanly.
- Warnings are logged when robots.txt cannot be fetched (fail-open) or
  contains a malformed `Crawl-delay`.
- Logging configuration keys documented in `linktadoru.yml.example`.
- Dependabot configuration for Go module and GitHub Actions updates.

### Removed
- Dead code: unused `GetProcessingItems` storage method.

## [0.8.7] - 2026-06-27

### Fixed
- `include_patterns` / `exclude_patterns` had no effect (#46): discovered
  links were queued unconditionally, bypassing the URL filters. Link-graph
  nodes are now recorded as `discovered` and only promoted to the crawl queue
  when they pass the filters. Existing databases are migrated in place.
- Crawler no longer hangs on network-errored seeds, malformed URLs, or stale
  `processing` rows left by an interrupted run.
- Fixed Makefile target issues (#43).

## [0.8.6] - 2025-08-10

### Fixed
- URL filtering and same-host validation logic (#38).

## [0.8.0 – 0.8.5] - 2025-08-07 – 2025-08-10

- Authentication support (basic / bearer / API key), custom HTTP headers,
  logging with rotation, CI/release pipeline hardening, and assorted fixes.

## [0.5.0 – 0.7.2] - 2025-08-01 – 2025-08-07

- Initial public iterations: concurrent queue-based crawler with SQLite
  storage, robots.txt support, URL filtering, and link-graph analysis.
