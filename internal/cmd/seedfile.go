package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// maxSeedLineBytes bounds a single line of a seed file. bufio.Scanner needs an
// explicit ceiling; without one a file with no newlines (a binary blob picked
// by mistake, a truncated download) would be pulled into memory whole.
const maxSeedLineBytes = 64 * 1024

// stdinSeedPath is the --seed-file value that means "read standard input".
const stdinSeedPath = "-"

// utf8BOM is emitted at the start of files by several Windows editors. It is
// invisible, so a seed file that carries one would otherwise fail on its first
// URL with a confusing error.
const utf8BOM = "\uFEFF"

// readSeedURLs reads seed URLs from path, one per line. A path of "-" reads
// from stdin, which is passed in rather than taken from os.Stdin so callers
// (and tests) control the source.
//
// Surrounding whitespace is trimmed, which also absorbs the CR of a CRLF file.
// Blank lines and lines whose first non-space character is '#' are skipped, so
// a generated list can carry comments. A read failure or an over-long line is
// reported rather than silently truncating the list: a partial seed list looks
// like a successful crawl of the wrong scope.
func readSeedURLs(path string, stdin io.Reader) ([]string, error) {
	var r io.Reader
	if path == stdinSeedPath {
		r = stdin
	} else {
		f, err := os.Open(path) // #nosec G304 -- the path is supplied by the operator running the crawler
		if err != nil {
			return nil, fmt.Errorf("failed to open seed file: %w", err)
		}
		defer func() { _ = f.Close() }()
		r = f
	}

	scanner := bufio.NewScanner(r)
	// One byte over the limit, so a line of exactly maxSeedLineBytes still has
	// room for its delimiter and is accepted rather than reported as too long.
	scanner.Buffer(make([]byte, 0, 4096), maxSeedLineBytes+1)

	var urls []string
	lines := 0
	for scanner.Scan() {
		line := scanner.Text()
		if lines == 0 {
			line = strings.TrimPrefix(line, utf8BOM)
		}
		lines++

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
	}

	if err := scanner.Err(); err != nil {
		// Err reports the failure of the line after the last one scanned.
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, fmt.Errorf("seed input %s: line %d exceeds the %d byte limit",
				seedSourceName(path), lines+1, maxSeedLineBytes)
		}
		return nil, fmt.Errorf("failed to read seed input %s: %w", seedSourceName(path), err)
	}

	return urls, nil
}

// seedSourceName renders a seed source for error messages.
func seedSourceName(path string) string {
	if path == stdinSeedPath {
		return "(standard input)"
	}
	return path
}
