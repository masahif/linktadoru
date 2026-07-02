# Changelog

All notable changes to LinkTadoru are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Fixed
- Retry mechanism never fired: the retryable-error filter matched error types
  that were never written. Transient `network_error` failures are now retried
  (up to 3 times); deterministic failures are not.
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
