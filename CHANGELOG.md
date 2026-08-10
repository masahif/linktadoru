# Changelog

All notable changes to LinkTadoru are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.12.0] - 2026-08-10

### Removed
- **Remove `follow_external_hosts` for v0.12.0.** The old
  `--follow-external-hosts` flag is rejected as an unknown flag. The old YAML
  key and `LT_FOLLOW_EXTERNAL_HOSTS` environment variable are ignored and have
  no effect. Add trusted cross-origin ranges explicitly with
  `include_patterns`, for example: `^https://trusted\.example(?:/.*)?$`.
- Existing databases may mark pending URLs outside the current URL policy as
  skipped. Re-add such URLs as explicit seeds if they need to be crawled.

## [0.11.0] - 2026-08-07

Versioning note: v0.10.0 was withdrawn. This release uses v0.11.0 instead of
reusing a version number that had already been published.

### Compatibility and upgrade notes
- **URL include semantics changed from filtering to authorization.** Persisted
  depth-0 seed origins are always allowed; `include_patterns` now add absolute
  full-URL ranges, and `exclude_patterns` subtract from the combined set.
  Includes are matched against the entire absolute URL instead of as
  substrings, so patterns that relied on partial matches must state their full
  range. Relative includes such as `/products/` are rejected at startup. During
  discovered-link admission, existing absolute includes for non-seed origins
  were previously inert behind the seed-host check; they now actively
  authorize those ranges.
- **`follow_external_hosts: true` now performs external fetches.** In v0.9.2,
  the parent-host link gate left discovered external URLs graph-only even when
  this option was enabled. It now permits crawling every URL with an allowed
  scheme; `include_patterns` do not narrow this allow-all mode, while
  `exclude_patterns` still apply. Review existing `true` configurations before
  upgrading because their network reach can expand substantially, including to
  private or link-local destinations linked by a crawled page.
- **Bounded crawl depth is first-admission depth, not shortest-path BFS depth.**
  Seeds are depth 0 and a newly queued child is `parent depth + 1`. All depths
  use the asynchronous queue without a layer barrier. For `max_depth >= 2`, a
  URL first admitted through a longer path can prevent descendants from being
  fetched even if a shorter path is discovered later; response timing can
  therefore affect the fetched set.
- **Existing databases receive a nullable `pages.depth` column.** A run stops
  before network access when unfinished legacy rows have unknown depth. Supply
  those URLs explicitly as seeds, or use a fresh database. Changing seeds or
  `max_depth` in a reused database does not retroactively recompute the depth of
  other admitted or terminal rows; use a fresh database for independently
  comparable snapshots.
- **Explicit seeds now mean an explicit refresh.** Re-supplying an existing URL
  queues it at depth 0, resets its retry budget, and clears its prior page/error
  observation. Normal duplicate discovery still does not re-fetch terminal
  rows. Command-line URL arguments and `--seed-file` also take precedence over
  `seed_urls` loaded from configuration.
- **All explicit seeds are retained even when their count exceeds `--limit`.**
  v0.9.2 discarded seeds beyond the current limit during initialization. They
  are now queued before workers start, so seeds not reached in the current run
  remain pending for a later resume.
- **Credential trust is invocation-specific.** Authentication and configured
  custom headers are sent only to origins supplied as seeds in the current
  invocation. A seedless resume with credentials configured fails closed and
  asks for the seed list again.

### Added
- `--seed-file` reads generated seed lists from a file or standard input (#67).
- SQLite persists first-discovery depth. `max_depth: N` accepts any non-negative
  N, prioritizes shallower queued work, and supports depth-preserving resume
  (#68, #72).
- URL authorization now uses one policy: exact persisted seed origins plus
  full-URL include regexes, minus exclude regexes. Includes can explicitly add
  cross-origin ranges; relative legacy includes fail with a migration message
  (#79).

### Changed
- Runs without explicit seeds now drain resumable database work, including
  bounded crawls, or exit successfully when no work remains.
- Outgoing external links are retained as graph-only `discovered` rows even
  when their URLs are not authorized for fetching.
- In multi-seed crawls, links between persisted seed origins can now be fetched;
  the old parent-host `internal` link gate no longer narrows the shared policy.
- Dependencies: `golang.org/x/net` 0.56.0 → 0.57.0 and
  `modernc.org/sqlite` 1.38.2 → 1.56.0. GitHub Actions were updated to
  `setup-go` v7, `upload-artifact` v7, and `softprops/action-gh-release` v3.

### Security
- The redirect protections released in v0.9.2 for
  [GHSA-2692-7f24-52v6](https://github.com/masahif/linktadoru/security/advisories/GHSA-2692-7f24-52v6)
  remain in force: every redirect hop is checked by the same URL policy before
  contact, and cross-origin credentials and configured secret headers are
  removed.
- URL policy is evaluated before robots.txt lookup or any other network request;
  a denied queued URL is stored as `skipped` without contacting its origin.
- Credentials are never sent to origins authorized only by `include_patterns`,
  and once an A-B-A redirect chain leaves its initial origin they do not
  reappear when the chain returns to A.
- Seed URLs containing userinfo are rejected; configure credentials separately.

### Fixed
- HTTP 408, 429, 500, 502, 503, and 504 responses are now retained as errors
  with their response status, headers, timing, and size, then retried after
  normal queue work, up to 3 total attempts per URL. Permanent responses such
  as 403, 404, and 410 remain completed observations and are not retried (#71).

### Known limitations
- Transient retries use the normal per-domain delay and robots.txt
  `Crawl-delay`, but do not honor `Retry-After` or add a separate backoff.
- A page that succeeds after a retry is marked `completed`, but its
  `last_error_type`, error message, and retry count remain as attempt history;
  use `pages.status` as the final outcome.
- URL regex matching uses the stored absolute URL text and does not
  percent-decode it. For example, `/ika/` does not match `/%69ka/`.
- `follow_external_hosts: true` is an explicit allow-all compatibility mode;
  there is no separate private-network, loopback, link-local, or cloud-metadata
  address deny floor.

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
