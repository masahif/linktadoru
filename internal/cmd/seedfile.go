package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// readSeedURLs reads one URL per line. Blank lines and comments are ignored;
// "-" uses stdin so generated lists never need to pass through argv.
func readSeedURLs(path string, stdin io.Reader) ([]string, error) {
	var r io.Reader = stdin
	if path != "-" {
		f, err := os.Open(path) // #nosec G304 -- the operator explicitly supplies this path
		if err != nil {
			return nil, fmt.Errorf("open seed file: %w", err)
		}
		defer func() { _ = f.Close() }()
		r = f
	}

	var urls []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read seed file: %w", err)
	}
	return urls, nil
}
