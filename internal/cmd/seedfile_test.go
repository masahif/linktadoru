package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/masahif/linktadoru/internal/config"
)

// writeSeedFile writes content to a temp file and returns its path.
func writeSeedFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "seeds.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write seed file: %v", err)
	}
	return path
}

func equalURLs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestReadSeedURLsFromFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "one URL per line",
			content: "https://a.example.com\nhttps://b.example.com\n",
			want:    []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name:    "blank lines and comments are skipped",
			content: "# target list\n\nhttps://a.example.com\n\n  # indented comment\nhttps://b.example.com\n",
			want:    []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name:    "surrounding whitespace is trimmed",
			content: "  https://a.example.com  \n\thttps://b.example.com\t\n",
			want:    []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name:    "CRLF line endings",
			content: "https://a.example.com\r\nhttps://b.example.com\r\n",
			want:    []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name:    "leading UTF-8 BOM is stripped",
			content: "\uFEFFhttps://a.example.com\nhttps://b.example.com\n",
			want:    []string{"https://a.example.com", "https://b.example.com"},
		},
		{
			name:    "no trailing newline",
			content: "https://a.example.com",
			want:    []string{"https://a.example.com"},
		},
		{
			name:    "fragments are not mistaken for comments",
			content: "https://a.example.com/page#section\n",
			want:    []string{"https://a.example.com/page#section"},
		},
		{
			name:    "empty file yields no URLs",
			content: "",
			want:    nil,
		},
		{
			name:    "comments only yields no URLs",
			content: "# nothing here\n\n",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readSeedURLs(writeSeedFile(t, tt.content), nil)
			if err != nil {
				t.Fatalf("readSeedURLs failed: %v", err)
			}
			if !equalURLs(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadSeedURLsFromStdin(t *testing.T) {
	in := strings.NewReader("# from a pipe\nhttps://a.example.com\n\nhttps://b.example.com\n")

	got, err := readSeedURLs("-", in)
	if err != nil {
		t.Fatalf("readSeedURLs failed: %v", err)
	}
	want := []string{"https://a.example.com", "https://b.example.com"}
	if !equalURLs(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A missing or unreadable file must stop the run rather than silently crawling
// an empty seed list.
func TestReadSeedURLsMissingFileFails(t *testing.T) {
	_, err := readSeedURLs(filepath.Join(t.TempDir(), "does-not-exist.txt"), nil)
	if err == nil {
		t.Fatal("expected an error for a missing seed file")
	}
	if !strings.Contains(err.Error(), "failed to open seed file") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// An over-long line must be reported. Silently dropping it would truncate the
// seed list and make a partial crawl look complete.
func TestReadSeedURLsOversizedLineFails(t *testing.T) {
	huge := "https://example.com/" + strings.Repeat("x", maxSeedLineBytes)
	content := "https://a.example.com\n" + huge + "\n"

	_, err := readSeedURLs(writeSeedFile(t, content), nil)
	if err == nil {
		t.Fatal("expected an error for an over-long line")
	}
	if !strings.Contains(err.Error(), "line 2") || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should identify the offending line and the limit, got: %v", err)
	}
}

// A line exactly at the limit is still accepted.
func TestReadSeedURLsAcceptsLineAtLimit(t *testing.T) {
	url := "https://example.com/" + strings.Repeat("x", maxSeedLineBytes-len("https://example.com/"))
	if len(url) != maxSeedLineBytes {
		t.Fatalf("test setup: line length %d, want %d", len(url), maxSeedLineBytes)
	}

	got, err := readSeedURLs(writeSeedFile(t, url+"\n"), nil)
	if err != nil {
		t.Fatalf("readSeedURLs failed at exactly the limit: %v", err)
	}
	if !equalURLs(got, []string{url}) {
		t.Errorf("got %d URLs, want the single line back", len(got))
	}
}

// seedCmd builds a command carrying the flags applySeedURLs reads. An empty
// seedFile leaves the flag unset, as if it had not been passed at all.
func seedCmd(t *testing.T, seedFile string, stdin string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("seed-file", "", "")
	if seedFile != "" {
		if err := cmd.Flags().Set("seed-file", seedFile); err != nil {
			t.Fatalf("failed to set seed-file: %v", err)
		}
	}
	cmd.SetIn(strings.NewReader(stdin))
	return cmd
}

// seedCmdExplicitEmpty builds a command where --seed-file was passed with an
// empty value, as an unset shell variable expands to.
func seedCmdExplicitEmpty(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("seed-file", "", "")
	if err := cmd.Flags().Set("seed-file", ""); err != nil {
		t.Fatalf("failed to set seed-file: %v", err)
	}
	return cmd
}

// The command line must win over seed_urls in the config file. Before this fix
// viper.Unmarshal overwrote the positional arguments with the config values.
func TestApplySeedURLsPrecedence(t *testing.T) {
	fromConfig := []string{"https://from-config.example.com"}

	t.Run("positional arguments beat the config file", func(t *testing.T) {
		cfg := &config.CrawlConfig{SeedURLs: fromConfig}

		if err := applySeedURLs(seedCmd(t, "", ""), []string{"https://from-args.example.com"}, cfg); err != nil {
			t.Fatalf("applySeedURLs failed: %v", err)
		}
		if !equalURLs(cfg.SeedURLs, []string{"https://from-args.example.com"}) {
			t.Errorf("got %q, want the positional argument", cfg.SeedURLs)
		}
	})

	t.Run("seed file beats the config file", func(t *testing.T) {
		cfg := &config.CrawlConfig{SeedURLs: fromConfig}
		path := writeSeedFile(t, "https://from-file.example.com\n")

		if err := applySeedURLs(seedCmd(t, path, ""), nil, cfg); err != nil {
			t.Fatalf("applySeedURLs failed: %v", err)
		}
		if !equalURLs(cfg.SeedURLs, []string{"https://from-file.example.com"}) {
			t.Errorf("got %q, want the seed file entry", cfg.SeedURLs)
		}
	})

	t.Run("stdin beats the config file", func(t *testing.T) {
		cfg := &config.CrawlConfig{SeedURLs: fromConfig}

		if err := applySeedURLs(seedCmd(t, "-", "https://from-stdin.example.com\n"), nil, cfg); err != nil {
			t.Fatalf("applySeedURLs failed: %v", err)
		}
		if !equalURLs(cfg.SeedURLs, []string{"https://from-stdin.example.com"}) {
			t.Errorf("got %q, want the stdin entry", cfg.SeedURLs)
		}
	})

	t.Run("config file is kept when the command line names no seeds", func(t *testing.T) {
		cfg := &config.CrawlConfig{SeedURLs: fromConfig}

		if err := applySeedURLs(seedCmd(t, "", ""), nil, cfg); err != nil {
			t.Fatalf("applySeedURLs failed: %v", err)
		}
		if !equalURLs(cfg.SeedURLs, fromConfig) {
			t.Errorf("got %q, want the config file value untouched", cfg.SeedURLs)
		}
	})

	t.Run("an empty seed file clears the config seeds", func(t *testing.T) {
		// The run then joins the existing no-seed path: resume from the queue,
		// or the "no URLs provided" error when there is no database.
		cfg := &config.CrawlConfig{SeedURLs: fromConfig}

		if err := applySeedURLs(seedCmd(t, writeSeedFile(t, "# all comments\n"), ""), nil, cfg); err != nil {
			t.Fatalf("applySeedURLs failed: %v", err)
		}
		if len(cfg.SeedURLs) != 0 {
			t.Errorf("got %q, want no seeds", cfg.SeedURLs)
		}
	})
}

// Combining the two input forms is ambiguous, so it is refused outright.
func TestApplySeedURLsRejectsBothInputs(t *testing.T) {
	cfg := &config.CrawlConfig{}
	path := writeSeedFile(t, "https://from-file.example.com\n")

	err := applySeedURLs(seedCmd(t, path, ""), []string{"https://from-args.example.com"}, cfg)
	if err == nil {
		t.Fatal("expected an error when --seed-file and URL arguments are combined")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// A read failure must surface from applySeedURLs, not be swallowed into an
// empty seed list.
func TestApplySeedURLsPropagatesReadError(t *testing.T) {
	cfg := &config.CrawlConfig{SeedURLs: []string{"https://from-config.example.com"}}
	missing := filepath.Join(t.TempDir(), "does-not-exist.txt")

	if err := applySeedURLs(seedCmd(t, missing, ""), nil, cfg); err == nil {
		t.Fatal("expected the read error to propagate")
	}
}

// End-to-end through runCrawler: a seed file naming no URLs must behave like a
// run with no seeds at all, i.e. hit the existing startup validation.
func TestRunCrawlerEmptySeedFileJoinsNoSeedPath(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	dbPath := filepath.Join(t.TempDir(), "nonexistent.db")

	cmd := seedCmd(t, writeSeedFile(t, "# nothing to crawl\n"), "")
	cmd.Flags().Bool("show-config", false, "")
	cmd.Flags().String("database", dbPath, "")
	if err := viper.BindPFlag("database_path", cmd.Flags().Lookup("database")); err != nil {
		t.Fatalf("failed to bind database flag: %v", err)
	}

	err := runCrawler(cmd, []string{})
	if err == nil {
		t.Fatal("expected the no-URLs startup validation to fire")
	}
	if !strings.Contains(err.Error(), "no URLs provided and no existing database found") {
		t.Errorf("expected the existing no-seed error, got: %v", err)
	}
}

// An empty --seed-file value must stop the run. Falling back to the config
// file's seed_urls would crawl a different set of hosts than the caller asked
// for, which a CI job expanding an unset variable would never notice.
func TestApplySeedURLsRejectsEmptySeedFileValue(t *testing.T) {
	cfg := &config.CrawlConfig{SeedURLs: []string{"https://from-config.example.com"}}

	err := applySeedURLs(seedCmdExplicitEmpty(t), nil, cfg)
	if err == nil {
		t.Fatal("expected an error for an explicitly empty --seed-file value")
	}
	if !strings.Contains(err.Error(), "needs a path") {
		t.Errorf("unexpected error message: %v", err)
	}
	if !equalURLs(cfg.SeedURLs, []string{"https://from-config.example.com"}) {
		t.Errorf("config seeds must be left untouched, got %q", cfg.SeedURLs)
	}
}
